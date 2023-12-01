package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/unnamedtiger/aura/api"
)

var runnerCheckins map[string]time.Time
var tagCheckins map[string]time.Time

func checkAdminAuth(auth string) (bool, error) {
	admin, err := LoadAdmin()
	if err != nil {
		return false, err
	}
	return CompareHashAndPassword(admin.Auth, auth)
}

func checkJobAuth(jobAuth []byte, auth string) (bool, error) {
	if strings.HasPrefix(auth, PrefixJob) {
		return CompareHashAndPassword(jobAuth, auth)
	} else if strings.HasPrefix(auth, PrefixAdmin) {
		return checkAdminAuth(auth)
	} else {
		return false, nil
	}
}

func checkProjectAuth(projectAuth []byte, auth string) (bool, error) {
	if strings.HasPrefix(auth, PrefixProject) {
		return CompareHashAndPassword(projectAuth, auth)
	} else if strings.HasPrefix(auth, PrefixAdmin) {
		return checkAdminAuth(auth)
	} else {
		return false, nil
	}
}

func checkRunnerAuth(runnerAuth []byte, auth string) (bool, error) {
	if strings.HasPrefix(auth, PrefixRunner) {
		return CompareHashAndPassword(runnerAuth, auth)
	} else if strings.HasPrefix(auth, PrefixAdmin) {
		return checkAdminAuth(auth)
	} else {
		return false, nil
	}
}

func respond(w http.ResponseWriter, code int, data any) {
	respBody, err := json.Marshal(data)
	if err != nil {
		log.Println(err)
		w.WriteHeader(500)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"code\":500,\"message\":\"internal server error\"}"))
		return
	}
	w.WriteHeader(code)
	w.Header().Add("Content-Type", "application/json")
	w.Write(respBody)
}

func respondError(w http.ResponseWriter, code int, msg string) {
	respond(w, code, api.Status{Code: code, Message: msg})
}

func RouteApiJob(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	var req api.JobRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusBadRequest, "unable to unmarshal json object")
		return
	}
	runner, err := FindRunnerByName(req.Name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			respondError(w, http.StatusBadRequest, "unknown runner")
			return
		}
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		respondError(w, http.StatusUnauthorized, "missing authorization header")
		return
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk, err := checkRunnerAuth(runner.Auth, authHeader)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !authOk {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	status := StatusFailed
	if req.ExitCode == 0 {
		status = StatusSucceeded
	}
	err = MarkJobDone(req.Id, status, req.ExitCode, t)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	go handlePrecedingJobCompleted(req.Id, status, t)
	respond(w, http.StatusOK, api.JobResponse{})
}

func handlePrecedingJobCompleted(jobId int64, status int, now time.Time) {
	if status == StatusFailed || status == StatusCancelled {
		succedingJobIds, err := FindSuccedingJobIds(jobId)
		if err != nil {
			log.Println(err)
			return
		}
		for _, succedingJobId := range succedingJobIds {
			err = MarkJobDone(succedingJobId, StatusCancelled, 0, now)
			if err != nil {
				log.Println(err)
				return
			}
			handlePrecedingJobCompleted(succedingJobId, StatusCancelled, now)
		}
	}
	err := MarkPrecedingJobCompleted(jobId)
	if err != nil {
		log.Println(err)
		return
	}

	job, err := LoadJob(jobId)
	if err != nil {
		log.Println(err)
		return
	}
	UpdateEntityStatus(job.EntityId)
}

func RouteApiRunner(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	var req api.RunnerRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusBadRequest, "unable to unmarshal json object")
		return
	}
	runner, err := FindRunnerByName(req.Name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			respondError(w, http.StatusBadRequest, "unknown runner")
			return
		}
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		respondError(w, http.StatusUnauthorized, "missing authorization header")
		return
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk, err := checkRunnerAuth(runner.Auth, authHeader)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	if !authOk {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	runnerCheckins[req.Name] = t

	candidates := []int64{}
	for _, tag := range req.Tags {
		tagCheckins[tag] = t
		limit := int64(req.Limit - len(candidates))
		if limit > 0 {
			jobIds, err := FindJobsForRunner(tag, limit, t)
			if err != nil {
				log.Println(err)
				respondError(w, http.StatusInternalServerError, "internal server error")
				return
			}
			candidates = append(candidates, jobIds...)
		}
	}
	jobs := []api.RunnerResponseJob{}
	for _, candidate := range candidates {
		pass, hash, err := GenerateRandom(PrefixJob)
		if err != nil {
			log.Println(err)
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		jobObj, err := ReserveJobForRunner(candidate, hash, runner.Id, t)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			log.Println(err)
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		entity, err := LoadEntity(jobObj.EntityId)
		if err != nil {
			log.Println(err)
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		project, err := LoadProject(entity.ProjectId)
		if err != nil {
			log.Println(err)
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		job := api.RunnerResponseJob{
			Id:        jobObj.Id,
			Project:   project.Name,
			EntityKey: entity.Key,
			EntityVal: entity.Val,
			Name:      jobObj.Name,
			JobKey:    pass,
			Cmd:       jobObj.Cmd,
			Env:       jobObj.Env,
			Tag:       jobObj.Tag,
		}
		jobs = append(jobs, job)
		if len(jobs) >= req.Limit {
			break
		}
	}

	respond(w, http.StatusOK, api.RunnerResponse{Jobs: jobs})
}

var allowedStorageRegex = regexp.MustCompile(`^\d+/log$`)

func RouteApiStorage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/storage/")
	if !allowedStorageRegex.MatchString(p) {
		respondError(w, http.StatusBadRequest, "invalid storage path")
		return
	}
	jobIdString := p[:strings.Index(p, "/")]
	jobId, err := strconv.ParseInt(jobIdString, 10, 64)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}
	job, err := LoadJob(jobId)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			respondError(w, http.StatusBadRequest, "unknown job")
			return
		}
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		respondError(w, http.StatusUnauthorized, "missing authorization header")
		return
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk := false
	if strings.HasPrefix(authHeader, PrefixRunner) {
		runner, err := LoadRunner(job.Runner)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				respondError(w, http.StatusBadRequest, "unknown runner")
				return
			}
			log.Println(err)
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		authOk, err = checkRunnerAuth(runner.Auth, authHeader)
		if err != nil {
			log.Println(err)
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	} else {
		authOk, err = checkJobAuth(job.Auth, authHeader)
		if err != nil {
			log.Println(err)
			respondError(w, http.StatusInternalServerError, "internal server error")
			return
		}
	}
	if !authOk {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	p = path.Join("artifacts", p)
	err = os.MkdirAll(path.Dir(p), os.ModePerm)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	file, err := os.Create(p)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	_, err = io.Copy(file, r.Body)
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	err = file.Close()
	if err != nil {
		log.Println(err)
		respondError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	respond(w, http.StatusOK, api.StorageResponse{})
}
