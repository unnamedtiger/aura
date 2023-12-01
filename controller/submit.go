package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/unnamedtiger/aura/api"
)

// NOTE: ProjectName from api.SubmitRequest is ignored, ProjectId is used instead
type Submission struct {
	api.SubmitRequest
	ProjectId int64
}

type SubmitError struct {
	code int
	msg  string
	err  error
}

type SubmitEndpoint interface {
	HandleRequest(r *http.Request) (Submission, *SubmitError)
	UpdateEntityStatus(project Project, entity EntityOrCollection)
}

var submitEndpoints map[string]SubmitEndpoint

type GeneralConfig struct {
	BaseUrl string `json:"baseUrl"`
}

func InitializeSubmitEndpoints() {
	var gcfg GeneralConfig
	var cfg map[string]json.RawMessage
	submitEndpoints = map[string]SubmitEndpoint{}

	_, err := os.Stat("config.json")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Fatalln(err)
		}
	} else {
		data, err := os.ReadFile("config.json")
		if err != nil {
			log.Fatalln(err)
		}
		err = json.Unmarshal(data, &gcfg)
		if err != nil {
			log.Fatalln(err)
		}
		err = json.Unmarshal(data, &cfg)
		if err != nil {
			log.Fatalln(err)
		}
	}
	baseUrl, err := url.Parse(gcfg.BaseUrl)
	if err != nil {
		log.Fatalln(err)
	}
	gcfg.BaseUrl = fmt.Sprintf("%s://%s", baseUrl.Scheme, baseUrl.Host)

	submitEndpoints[""] = NewSubmitEndpointGeneric()
	log.Printf("Initialized generic submit endpoint at /api/submit\n")

	cfgKeys := make([]string, 0, len(cfg))
	for k := range cfg {
		cfgKeys = append(cfgKeys, k)
	}
	sort.Strings(cfgKeys)

	for _, cfgKey := range cfgKeys {
		if cfgKey == "baseUrl" {
			continue
		} else if cfgKey == "darke" {
			submitEndpoints["darke"], err = NewSubmitEndpointDarke(cfg["darke"])
			if err != nil {
				log.Fatalln(err)
			}
			log.Printf("Initialized submit endpoint for Darke at /api/submit/darke\n")
		} else if cfgKey == "gitea" {
			submitEndpoints["gitea"], err = NewSubmitEndpointGitea(gcfg.BaseUrl, cfg["gitea"])
			if err != nil {
				log.Fatalln(err)
			}
			log.Printf("Initialized submit endpoint for Gitea at /api/submit/gitea\n")
		} else {
			log.Fatalf("Unknown integration '%s'\n", cfgKey)
		}
	}
}

func Submit(sub Submission) (int64, *SubmitError) {
	t := time.Now()
	if !slugRegex.MatchString(sub.Name) {
		return 0, &SubmitError{http.StatusBadRequest, "invalid name", nil}
	}

	entity, err := FindEntity(sub.ProjectId, sub.EntityKey, sub.EntityVal)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			if !slugRegex.MatchString(sub.EntityKey) {
				return 0, &SubmitError{http.StatusBadRequest, "invalid entityKey", nil}
			}
			if !slugRegex.MatchString(sub.EntityVal) {
				return 0, &SubmitError{http.StatusBadRequest, "invalid entityVal", nil}
			}
			err = CreateEntity(sub.ProjectId, sub.EntityKey, sub.EntityVal, t)
			if err != nil {
				return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
			}
			entity, err = FindEntity(sub.ProjectId, sub.EntityKey, sub.EntityVal)
			if err != nil {
				return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
			}
		} else {
			return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
	}

	earliestStart := t
	if sub.EarliestStart != nil {
		earliestStart = time.Unix(*sub.EarliestStart, 0)
	}
	jobId, err := CreateJob(entity.Id, sub.Name, t, earliestStart, sub.Cmd, sub.Env, sub.Tag)
	if err != nil {
		return 0, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}

	go submitPostprocessing(sub, jobId, t, entity.Id)
	return jobId, nil
}

func submitPostprocessing(sub Submission, jobId int64, t time.Time, entityId int64) {
	for key, value := range sub.Collections {
		coll, err := FindCollection(sub.ProjectId, key, value)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				err = CreateCollection(sub.ProjectId, key, value, t)
				if err != nil {
					log.Println(err)
					return
				}
			} else {
				log.Println(err)
				return
			}
			coll, err = FindCollection(sub.ProjectId, key, value)
			if err != nil {
				log.Println(err)
				return
			}
		}
		err = InsertEntityIntoCollection(coll.Id, entityId)
		if err != nil {
			log.Println(err)
			return
		}
	}

	for _, precedingJob := range sub.PrecedingJobs {
		_, err := LoadJob(precedingJob)
		if err != nil {
			if errors.Is(err, ErrNotFound) { // ignore invalid preceding jobs
				continue
			}
			log.Println(err)
			return
		}
		err = CreatePrecedingJob(precedingJob, jobId)
		if err != nil {
			log.Println(err)
			return
		}
		// NOTE: loading this job again to avoid race condition
		precedingJob, err := LoadJob(precedingJob)
		if err != nil {
			log.Println(err)
			return
		}
		if precedingJob.Status == StatusCancelled || precedingJob.Status == StatusFailed {
			err = MarkPrecedingJobCompleted(precedingJob.Id)
			if err != nil {
				log.Println(err)
				return
			}
			err = MarkJobDone(jobId, StatusCancelled, 0, t)
			if err != nil {
				log.Println(err)
				return
			}
		} else if precedingJob.Status == StatusSucceeded {
			err = MarkPrecedingJobCompleted(precedingJob.Id)
			if err != nil {
				log.Println(err)
				return
			}
		}
	}

	err := MarkJobCreated(jobId)
	if err != nil {
		log.Println(err)
		return
	}

	UpdateEntityStatus(entityId)
}

func RouteApiSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/submit")
	p = strings.TrimPrefix(p, "/")
	endpoint, found := submitEndpoints[p]
	if !found {
		respondError(w, http.StatusNotFound, "invalid submission endpoint")
		return
	}
	sub, err := endpoint.HandleRequest(r)
	if err != nil {
		log.Printf("%d %s %s\n", err.code, err.msg, err.err)
		respondError(w, err.code, err.msg)
		return
	}
	jobId, err := Submit(sub)
	if err != nil {
		log.Printf("%d %s %s\n", err.code, err.msg, err.err)
		respondError(w, err.code, err.msg)
		return
	}
	respond(w, http.StatusOK, api.SubmitResponse{Id: jobId})
}

type GenericJobConfig struct {
	Project string `json:"project"`
	Name    string `json:"name"`
	Cmd     string `json:"cmd"`
	Env     string `json:"env"`
	Tag     string `json:"tag"`
}

func HandleGenericJobConfig(cfg GenericJobConfig, entityKey string, entityVal string, collections map[string]string) (Submission, *SubmitError) {
	project, err := FindProjectBySlug(cfg.Project)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Submission{}, &SubmitError{http.StatusBadRequest, "unknown project", err}
		}
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}

	return Submission{
		SubmitRequest: api.SubmitRequest{
			Project:     project.Slug,
			EntityKey:   entityKey,
			EntityVal:   entityVal,
			Name:        cfg.Name,
			Cmd:         cfg.Cmd,
			Env:         cfg.Env,
			Tag:         cfg.Tag,
			Collections: collections,
		},
		ProjectId: project.Id,
	}, nil
}

func UpdateEntityStatus(entityId int64) {
	entity, err := LoadEntity(entityId)
	if err != nil {
		log.Println(err)
		return
	}
	project, err := LoadProject(entity.ProjectId)
	if err != nil {
		log.Println(err)
		return
	}
	for _, endpoint := range submitEndpoints {
		endpoint.UpdateEntityStatus(project, entity)
	}
}

func getEntityStatus(entityId int64) (string, string, error) {
	jobs, err := FindJobs(entityId)
	if err != nil {
		return "", "", err
	}
	queued := 0  // submitted and created
	running := 0 // started
	cancelled := 0
	succeeded := 0
	failed := 0
	for _, job := range jobs {
		switch job.Status {
		case StatusCreated:
			queued += 1
		case StatusStarted:
			running += 1
		case StatusCancelled:
			cancelled += 1
		case StatusSucceeded:
			succeeded += 1
		case StatusFailed:
			failed += 1
		case StatusSubmitted:
			queued += 1
		}
	}
	desc := []string{}
	if queued > 0 {
		desc = append(desc, fmt.Sprintf("%d queued", queued))
	}
	if running > 0 {
		desc = append(desc, fmt.Sprintf("%d running", running))
	}
	if cancelled > 0 {
		desc = append(desc, fmt.Sprintf("%d cancelled", cancelled))
	}
	if succeeded > 0 {
		desc = append(desc, fmt.Sprintf("%d succeeded", succeeded))
	}
	if failed > 0 {
		desc = append(desc, fmt.Sprintf("%d failed", failed))
	}
	state := ""
	if queued > 0 || running > 0 {
		state = "pending"
	} else if failed > 0 {
		state = "failure"
	} else {
		state = "success"
	}
	return strings.Join(desc, ", "), state, nil
}

// =============================================================================
// /api/submit
// =============================================================================

type SubmitEndpointGeneric struct {
}

func NewSubmitEndpointGeneric() SubmitEndpointGeneric {
	return SubmitEndpointGeneric{}
}

func (SubmitEndpointGeneric) HandleRequest(r *http.Request) (Submission, *SubmitError) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}
	var req api.SubmitRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusBadRequest, "unable to unmarshal json object", err}
	}
	project, err := FindProjectBySlug(req.Project)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Submission{}, &SubmitError{http.StatusBadRequest, "unknown project", err}
		}
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}

	authHeader := r.Header.Get("Authorization")
	if len(authHeader) == 0 {
		return Submission{}, &SubmitError{http.StatusUnauthorized, "missing authorization header", nil}
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authOk := false
	if strings.HasPrefix(authHeader, PrefixJob) && req.ParentJob != nil {
		parentJob, err := LoadJob(*req.ParentJob)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return Submission{}, &SubmitError{http.StatusBadRequest, "unknown parent job", err}
			}
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
		parentEntity, err := LoadEntity(parentJob.EntityId)
		if err != nil {
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
		if project.Id != parentEntity.ProjectId {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "project does not match parent job project", nil}
		}
		if req.EntityKey != parentEntity.Key {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "entityKey does not match parent job entityKey", nil}
		}
		if req.EntityVal != parentEntity.Val {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "entityVal does not match parent job entityVal", nil}
		}
		if len(req.Collections) > 0 {
			return Submission{}, &SubmitError{http.StatusUnauthorized, "may not set collections when using JOBKEY", nil}
		}

		authOk, err = checkJobAuth(parentJob.Auth, authHeader)
		if err != nil {
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
	} else {
		for key, value := range req.Collections {
			if !slugRegex.MatchString(key) {
				return Submission{}, &SubmitError{http.StatusBadRequest, "invalid collection key", nil}
			}
			if !slugRegex.MatchString(value) {
				return Submission{}, &SubmitError{http.StatusBadRequest, "invalid collection value", nil}
			}
		}

		authOk, err = checkProjectAuth(project.Auth, authHeader)
		if err != nil {
			return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
		}
	}
	if !authOk {
		return Submission{}, &SubmitError{http.StatusUnauthorized, "unauthorized", nil}
	}
	return Submission{req, project.Id}, nil
}

func (SubmitEndpointGeneric) UpdateEntityStatus(project Project, entity EntityOrCollection) {
	// no-op
}

// =============================================================================
// /api/submit/darke
// =============================================================================

type SubmitEndpointDarke struct {
	Repos map[string]GenericJobConfig `json:"repos"`
}

type SubmitRequestDarke struct {
	Owner   string
	Repo    string
	RefName string
	Commit  string
}

func NewSubmitEndpointDarke(cfgData json.RawMessage) (*SubmitEndpointDarke, error) {
	var endpoint SubmitEndpointDarke
	err := json.Unmarshal(cfgData, &endpoint)
	if err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func (e *SubmitEndpointDarke) HandleRequest(r *http.Request) (Submission, *SubmitError) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}
	var req SubmitRequestDarke
	err = json.Unmarshal(body, &req)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusBadRequest, "unable to unmarshal json object", err}
	}
	repo, found := e.Repos[fmt.Sprintf("%s/%s", req.Owner, req.Repo)]
	if !found {
		return Submission{}, &SubmitError{http.StatusBadRequest, "repository not configured", err}
	}
	collections := map[string]string{}
	collections["ref"] = req.RefName
	return HandleGenericJobConfig(repo, "commit", req.Commit, collections)
}

func (e *SubmitEndpointDarke) UpdateEntityStatus(project Project, entity EntityOrCollection) {
	// no-op
}

// =============================================================================
// /api/submit/gitea
// =============================================================================

type SubmitEndpointGitea struct {
	auraBaseUrl string

	Repos map[string]SubmitEndpointGiteaJobConfig `json:"repos"`

	ApiBaseUrl string `json:"apiBaseUrl"`
}

type SubmitEndpointGiteaJobConfig struct {
	GenericJobConfig

	Authorization string `json:"authorization"`
	Secret        string `json:"secret"`
}

type SubmitRequestGitea struct {
	Ref   string `json:"ref"`
	After string `json:"after"`

	Repository SubmitRequestGiteaRepository `json:"repository"`
}

type SubmitRequestGiteaRepository struct {
	FullName string `json:"full_name"`
}

func NewSubmitEndpointGitea(auraBaseUrl string, cfgData json.RawMessage) (*SubmitEndpointGitea, error) {
	var endpoint SubmitEndpointGitea
	err := json.Unmarshal(cfgData, &endpoint)
	if err != nil {
		return nil, err
	}
	endpoint.auraBaseUrl = auraBaseUrl
	return &endpoint, nil
}

func (e *SubmitEndpointGitea) HandleRequest(r *http.Request) (Submission, *SubmitError) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}
	var req SubmitRequestGitea
	err = json.Unmarshal(body, &req)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusBadRequest, "unable to unmarshal json object", err}
	}
	repo, found := e.Repos[req.Repository.FullName]
	if !found {
		return Submission{}, &SubmitError{http.StatusBadRequest, "repository not configured", err}
	}
	signatureHeader := r.Header.Get("X-Hub-Signature-256")
	if signatureHeader == "" {
		return Submission{}, &SubmitError{http.StatusUnauthorized, "missing signature", err}
	}
	valid, err := validateWebhook(signatureHeader, repo.Secret, body)
	if err != nil {
		return Submission{}, &SubmitError{http.StatusInternalServerError, "internal server error", err}
	}
	if !valid {
		return Submission{}, &SubmitError{http.StatusUnauthorized, "invalid signature", err}
	}

	refParts := strings.Split(req.Ref, "/")
	refName := refParts[len(refParts)-1]
	collections := map[string]string{}
	collections["ref"] = refName
	return HandleGenericJobConfig(repo.GenericJobConfig, "commit", req.After, collections)
}

func (e *SubmitEndpointGitea) UpdateEntityStatus(project Project, entity EntityOrCollection) {
	if e.ApiBaseUrl != "" && entity.Key != "commit" {
		return
	}
	for key, repo := range e.Repos {
		if repo.Project == project.Slug {
			type reqObject struct {
				Context     string `json:"context"`     // label of who is providing this status
				Description string `json:"description"` // high-level summary
				State       string `json:"state"`       // "pending", "success", "error", or "failure"
				TargetUrl   string `json:"target_url"`  // full URL to build output
			}
			description, state, err := getEntityStatus(entity.Id)
			if err != nil {
				log.Println(err)
				return
			}
			targetUrl := fmt.Sprintf("%s/p/%s/%s/%s", e.auraBaseUrl, project.Slug, entity.Key, entity.Val)
			reqObj := reqObject{Context: "Aura", Description: description, State: state, TargetUrl: targetUrl}

			reqData, err := json.Marshal(reqObj)
			if err != nil {
				log.Println(err)
				return
			}
			url := fmt.Sprintf("%s/repos/%s/statuses/%s", e.ApiBaseUrl, key, entity.Val)
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(reqData))
			if err != nil {
				log.Println(err)
				return
			}
			if e.ApiBaseUrl == "https://api.github.com" {
				req.Header.Set("Accept", "application/vnd.github+json")
				req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", repo.Authorization)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Println(err)
				return
			}
			if resp.StatusCode != http.StatusCreated {
				log.Println("got status " + resp.Status)
			}
			return
		}
	}
}

func validateWebhook(signature string, secret string, payload []byte) (bool, error) {
	if !strings.HasPrefix(signature, "sha256=") {
		return false, nil
	}
	signature = signature[7:]
	if secret == "" && signature == "" {
		return true, nil
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	sig := hex.EncodeToString(h.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(signature), []byte(sig)) == 1, nil
}
