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

	Tag string `json:"tag"`

	EarliestStart *int64 `json:"earliestStart"`
}

func ApiSubmit(req SubmitRequest) (ApiResponse, error) {
	t := time.Now()
	if !slugRegex.MatchString(req.Name) {
		return ApiResponse{Code: http.StatusBadRequest, Message: "invalid name"}, nil
	}
	project, err := FindProjectBySlug(req.Project)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ApiResponse{Code: http.StatusNotFound, Message: "project not found"}, nil
		}
		return ApiResponse{}, err
	}
	entity, err := FindEntity(project.Id, req.EntityKey, req.EntityVal)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if !slugRegex.MatchString(req.EntityKey) {
				return ApiResponse{Code: http.StatusBadRequest, Message: "invalid entityKey"}, nil
			}
			if !slugRegex.MatchString(req.EntityVal) {
				return ApiResponse{Code: http.StatusBadRequest, Message: "invalid entityVal"}, nil
			}
			err = CreateEntity(project.Id, req.EntityKey, req.EntityVal, t)
			if err != nil {
				return ApiResponse{}, err
			}
			entity, err = FindEntity(project.Id, req.EntityKey, req.EntityVal)
			if err != nil {
				return ApiResponse{}, err
			}
		} else {
			return ApiResponse{}, err
		}
	}

	earliestStart := t
	if req.EarliestStart != nil {
		earliestStart = time.Unix(*req.EarliestStart, 0)
	}
	err = CreateJob(entity.Id, req.Name, t, earliestStart, req.Tag)
	if err != nil {
		return ApiResponse{}, err
	}
	return ApiResponse{Code: http.StatusAccepted, Message: "job created"}, nil
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
		jobIds, err := FindJobsForRunner(tag, int64(req.Limit-len(candidates)))
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			log.Println(err)
			return
		}
		candidates = append(candidates, jobIds...)
		if len(candidates) >= req.Limit {
			break
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

	respBody, err := json.Marshal(jobs)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Add("Content-Type", "application/json")
	w.Write(respBody)
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
	resp, err := ApiSubmit(req)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	respBody, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		log.Println(err)
		return
	}
	w.WriteHeader(resp.Code)
	w.Header().Add("Content-Type", "application/json")
	w.Write(respBody)
}
