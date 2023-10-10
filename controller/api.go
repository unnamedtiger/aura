package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

var runnerCheckins map[string]time.Time
var tagCheckins map[string]time.Time

type ApiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JobRequest struct {
	Name     string `json:"name"`
	Id       int64  `json:"id"`
	ExitCode int64  `json:"exitCode"`
}

type RunnerRequest struct {
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
	Limit int      `json:"limit"`
}

type RunnerResponse struct {
	Jobs []RunnerResponseJob `json:"jobs"`
}

type RunnerResponseJob struct {
	Id        int64  `json:"id"`
	Project   string `json:"project"`
	EntityKey string `json:"entityKey"`
	EntityVal string `json:"entityVal"`
	Name      string `json:"name"`
	JobKey    string `json:"jobKey"`
	Cmd       string `json:"cmd"`
	Env       string `json:"env"`
	Tag       string `json:"tag"`
}

type SubmitRequest struct {
	Project   string `json:"project"`
	EntityKey string `json:"entityKey"`
	EntityVal string `json:"entityVal"`
	Name      string `json:"name"`
	Cmd       string `json:"cmd"`
	Env       string `json:"env"`
	Tag       string `json:"tag"`

	PrecedingJobs []int64 `json:"precedingJobs"`
	EarliestStart *int64  `json:"earliestStart"`
}

func ApiSubmit(req SubmitRequest) (int64, ApiResponse, error) {
	t := time.Now()
	if !slugRegex.MatchString(req.Name) {
		return 0, ApiResponse{Code: http.StatusBadRequest, Message: "invalid name"}, nil
	}
	project, err := FindProjectBySlug(req.Project)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, ApiResponse{Code: http.StatusNotFound, Message: "project not found"}, nil
		}
		return 0, ApiResponse{}, err
	}
	entity, err := FindEntity(project.Id, req.EntityKey, req.EntityVal)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if !slugRegex.MatchString(req.EntityKey) {
				return 0, ApiResponse{Code: http.StatusBadRequest, Message: "invalid entityKey"}, nil
			}
			if !slugRegex.MatchString(req.EntityVal) {
				return 0, ApiResponse{Code: http.StatusBadRequest, Message: "invalid entityVal"}, nil
			}
			err = CreateEntity(project.Id, req.EntityKey, req.EntityVal, t)
			if err != nil {
				return 0, ApiResponse{}, err
			}
			entity, err = FindEntity(project.Id, req.EntityKey, req.EntityVal)
			if err != nil {
				return 0, ApiResponse{}, err
			}
		} else {
			return 0, ApiResponse{}, err
		}
	}

	earliestStart := t
	if req.EarliestStart != nil {
		earliestStart = time.Unix(*req.EarliestStart, 0)
	}
	jobId, err := CreateJob(entity.Id, req.Name, t, earliestStart, req.Cmd, req.Env, req.Tag)
	if err != nil {
		return 0, ApiResponse{}, err
	}

	for _, precedingJob := range req.PrecedingJobs {
		_, err := LoadJob(precedingJob)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return 0, ApiResponse{Code: http.StatusBadRequest, Message: "invalid preceding job"}, nil
			}
			return 0, ApiResponse{}, err
		}
		err = CreatePrecedingJob(precedingJob, jobId)
		if err != nil {
			return 0, ApiResponse{}, err
		}
		// NOTE: loading this job again to avoid race condition
		precedingJob, err := LoadJob(precedingJob)
		if err != nil {
			return 0, ApiResponse{}, err
		}
		if precedingJob.Status == StatusCancelled || precedingJob.Status == StatusFailed {
			err = MarkPrecedingJobCompleted(precedingJob.Id)
			if err != nil {
				return 0, ApiResponse{}, err
			}
			err = MarkJobDone(jobId, StatusCancelled, 0, t)
			if err != nil {
				return 0, ApiResponse{}, err
			}
		} else if precedingJob.Status == StatusSucceeded {
			err = MarkPrecedingJobCompleted(precedingJob.Id)
			if err != nil {
				return 0, ApiResponse{}, err
			}
		}
	}
	return jobId, ApiResponse{Code: http.StatusAccepted, Message: "job created"}, nil
}

func checkProjectAuth(projectAuth []byte, auth string) (bool, error) {
	if strings.HasPrefix(auth, PrefixProject) {
		return CompareHashAndPassword(projectAuth, auth)
	} else {
		return false, nil
	}
}

func checkRunnerAuth(runnerAuth []byte, auth string) (bool, error) {
	if strings.HasPrefix(auth, PrefixRunner) {
		return CompareHashAndPassword(runnerAuth, auth)
	} else {
		return false, nil
	}
}

func respond(w http.ResponseWriter, code int, data any) {
	respBody, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	w.WriteHeader(code)
	w.Header().Add("Content-Type", "application/json")
	w.Write(respBody)
}

func RouteApiJob(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	var req JobRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Println(err)
		return
	}
	runner, err := FindRunnerByName(req.Name)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Println(err)
		return
	}
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk, err := checkRunnerAuth(runner.Auth, authHeader)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	if !authOk {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	status := StatusFailed
	if req.ExitCode == 0 {
		status = StatusSucceeded
	}
	err = MarkJobDone(req.Id, status, req.ExitCode, t)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	go handlePrecedingJobCompleted(req.Id, status, t)
	respond(w, http.StatusOK, ApiResponse{Code: http.StatusOK, Message: "recorded job completion"})
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
}

func RouteApiRunner(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	var req RunnerRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Println(err)
		return
	}
	runner, err := FindRunnerByName(req.Name)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Println(err)
		return
	}
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk, err := checkRunnerAuth(runner.Auth, authHeader)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	if !authOk {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
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
				http.Error(w, "internal server error", http.StatusInternalServerError)
				log.Println(err)
				return
			}
			candidates = append(candidates, jobIds...)
		}
	}
	jobs := []RunnerResponseJob{}
	for _, candidate := range candidates {
		pass, hash, err := GenerateRandom(PrefixJob)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		jobObj, err := ReserveJobForRunner(candidate, hash, runner.Id, t)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		entity, err := LoadEntity(jobObj.EntityId)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		project, err := LoadProject(entity.ProjectId)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		job := RunnerResponseJob{
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

	respond(w, http.StatusOK, RunnerResponse{Jobs: jobs})
}

func RouteApiStorage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/storage/")
	p = path.Join("artifacts", p)
	err := os.MkdirAll(path.Dir(p), os.ModePerm)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	file, err := os.Create(p)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	_, err = io.Copy(file, r.Body)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	err = file.Close()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	respond(w, http.StatusOK, ApiResponse{Code: http.StatusOK, Message: "saved file"})
}

func RouteApiSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	var req SubmitRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		log.Println(err)
		return
	}
	project, err := FindProjectBySlug(req.Project)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			http.Error(w, "project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk, err := checkProjectAuth(project.Auth, authHeader)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	if !authOk {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	jobId, resp, err := ApiSubmit(req)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	if resp.Code == http.StatusAccepted {
		type positiveResponse struct {
			Id int64 `json:"id"`
		}
		d := positiveResponse{Id: jobId}
		respond(w, resp.Code, d)
		return
	}
	respond(w, resp.Code, resp)
}
