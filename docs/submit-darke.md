# Integration for Darke Files

> ID: `darke`, Endpoint: `/api/submit/darke`

## Config

```json
"repos": {
    "user/repo": {
        "project": "foo",
        "name": "prepare",
        "cmd": "cihelper daf https://darke.example/user/repo",
        "env": "FOO=bar",
        "tag": "native,linux"
    }
}
```

* `user/repo` is the username plus reponame pair of the repository on your Darke server
* `project` is the project slug on Aura
* `name`, `cmd`, `env`, `tag` are the same as in the SubmitRequest

## Setup

* Use `darke files configure-webhook` to add a webhook to your repo, point it to this integration's endpoint
