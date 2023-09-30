package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"
)

var runnerCheckins map[string]time.Time
var tagCheckins map[string]time.Time

type ApiResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type JobRequest struct {
	Id       int64 `json:"id"`
	ExitCode int64 `json:"exitCode"`
}

type RunnerRequest struct {
	Name  string   `json:"name"`
	Tags  []string `json:"tags"`
	Limit int      `json:"limit"`
}

type SubmitRequest struct {
	Project   string `json:"project"`
	EntityKey string `json:"entityKey"`
	EntityVal string `json:"entityVal"`
	Name      string `json:"name"`
	Cmd       string `json:"cmd"`
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
	jobId, err := CreateJob(entity.Id, req.Name, t, earliestStart, req.Cmd, req.Tag)
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
	if r.Method != "POST" {
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
	respond(w, http.StatusAccepted, ApiResponse{Code: http.StatusAccepted, Message: "recorded job completion"})
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
	if r.Method != "POST" {
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
	jobs := []Job{}
	for _, candidate := range candidates {
		job, err := ReserveJobForRunner(candidate, runner.Id, t)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		jobs = append(jobs, job)
		if len(jobs) >= req.Limit {
			break
		}
	}

	respond(w, http.StatusOK, jobs)
}

func RouteApiSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
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
	// TODO: auth
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
