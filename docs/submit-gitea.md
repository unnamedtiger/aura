# Integration for Gitea

> ID: `gitea`, Endpoint: `/api/submit/gitea`

## Config

```json
"repos": {
    "user/repo": {
        "project": "foo",
        "name": "prepare",
        "cmd": "git clone https://gitea.example/user/repo && ./run.sh",
        "env": "FOO=bar",
        "tag": "native,linux"
    }
}
```

* `user/repo` is the username plus reponame pair of the repository on your Gitea
* `project` is the project name on Aura
* `name`, `cmd`, `env`, `tag` are the same as in the SubmitRequest

## Setup

* In your repository go to **Settings** > **Webhooks** and add a "Gitea" webhook
* Set the target URL to this integrations endpoint
* Select "POST" as method, "application/json" as content type, and triggering on push events
