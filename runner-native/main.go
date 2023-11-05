package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/unnamedtiger/aura/api"
)

type Config struct {
	Name       string   `json:"name"`
	Controller string   `json:"controller"`
	RunnerKey  string   `json:"runnerKey"`
	Tags       []string `json:"tags"`
}

func main() {
	data, err := os.ReadFile("config.json")
	if err != nil {
		log.Fatalln(err)
	}
	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatalln(err)
	}
	if len(cfg.Name) == 0 {
		log.Fatalln("invalid name")
	}
	if len(cfg.Controller) == 0 {
		log.Fatalln("invalid controller")
	}
	if len(cfg.RunnerKey) == 0 {
		log.Fatalln("invalid runnerKey")
	}
	if len(cfg.Tags) == 0 {
		log.Fatalln("invalid tags")
	}
	controllerUrl, err := url.Parse(cfg.Controller)
	if err != nil {
		log.Fatalln(err)
	}
	auraApi := api.New(controllerUrl)
	log.Printf("Starting runner %s...", cfg.Name)

	for {
		req := api.RunnerRequest{Name: cfg.Name, Tags: cfg.Tags, Limit: 1}
		reqData, err := json.Marshal(req)
		if err != nil {
			log.Fatalln(err)
		}
		httpReq, err := http.NewRequest(http.MethodPost, auraApi.Runner(), bytes.NewBuffer(reqData))
		if err != nil {
			log.Fatalln(err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+cfg.RunnerKey)
		respObj, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			log.Fatalln(err)
		}
		if respObj.StatusCode != http.StatusOK {
			log.Fatalln("got status " + respObj.Status)
		}
		var resp api.RunnerResponse
		err = json.NewDecoder(respObj.Body).Decode(&resp)
		if err != nil {
			log.Fatalln(err)
		}
		respObj.Body.Close()

		if len(resp.Jobs) > 0 {
			for _, job := range resp.Jobs {
				log.Printf("Running job %d...", job.Id)
				runJob(cfg, auraApi, job)
			}
		} else {
			log.Println("Sleeping...")
			time.Sleep(time.Minute)
		}
	}
}

func runJob(cfg Config, auraApi *api.AuraApi, job api.RunnerResponseJob) {
	exitCode := 0
	out := []byte{}
	parts, err := shlex.Split(job.Cmd)
	if err != nil {
		log.Println(err)
		exitCode = -1
	} else {
		partsArgs := []string{}
		if len(parts) > 1 {
			partsArgs = parts[1:]
		}
		wd := path.Join("w", fmt.Sprintf("%d", job.Id))
		err = os.MkdirAll(wd, os.ModePerm)
		if err != nil {
			log.Println(err)
			exitCode = -1
		} else {
			cmd := exec.Command(parts[0], partsArgs...)
			cmd.Dir = wd
			env := []string{}
			env = append(env, "CI=true")
			env = append(env, "AURA_CI=true")
			env = append(env, fmt.Sprintf("AURA_JOBID=%d", job.Id))
			env = append(env, fmt.Sprintf("AURA_JOBNAME=%s", job.Name))
			env = append(env, fmt.Sprintf("AURA_JOBKEY=%s", job.JobKey))
			env = append(env, fmt.Sprintf("AURA_PROJECT=%s", job.Project))
			env = append(env, fmt.Sprintf("AURA_ENTITYKEY=%s", job.EntityKey))
			env = append(env, fmt.Sprintf("AURA_ENTITYVAL=%s", job.EntityVal))
			env = append(env, strings.Split(job.Env, "\n")...)
			cmd.Env = env
			out, err = cmd.CombinedOutput()
			if err != nil {
				exitError, ok := err.(*exec.ExitError)
				if ok {
					exitCode = exitError.ExitCode()
				} else {
					log.Println(err)
					exitCode = -1
				}
			}
			log.Println(string(out))
			err = os.RemoveAll(wd)
			if err != nil {
				log.Println(err)
				exitCode = -1
			}
		}
	}

	req := api.JobRequest{Name: cfg.Name, Id: job.Id, ExitCode: int64(exitCode)}
	reqData, err := json.Marshal(req)
	if err != nil {
		log.Fatalln(err)
	}
	completeJobReq, err := http.NewRequest(http.MethodPost, auraApi.Job(), bytes.NewBuffer(reqData))
	if err != nil {
		log.Fatalln(err)
	}
	completeJobReq.Header.Set("Content-Type", "application/json")
	completeJobReq.Header.Set("Authorization", "Bearer "+cfg.RunnerKey)
	completeJobResp, err := http.DefaultClient.Do(completeJobReq)
	if err != nil {
		log.Fatalln(err)
	}
	if completeJobResp.StatusCode != http.StatusOK {
		log.Fatalln("got status " + completeJobResp.Status)
	}

	logUploadReq, err := http.NewRequest(http.MethodPost, auraApi.Storage(job.Id, "log"), bytes.NewBuffer(out))
	if err != nil {
		log.Fatalln(err)
	}
	logUploadReq.Header.Set("Content-Type", "application/json")
	logUploadReq.Header.Set("Authorization", "Bearer "+cfg.RunnerKey)
	logUploadResp, err := http.DefaultClient.Do(logUploadReq)
	if err != nil {
		log.Fatalln(err)
	}
	if logUploadResp.StatusCode != http.StatusOK {
		log.Fatalln("got status " + logUploadResp.Status)
	}
}
