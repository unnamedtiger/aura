# Integration for Gitea

> ID: `gitea`, Endpoint: `/api/submit/gitea`

## Config

```json
"apiBaseUrl": "http://gitea.example/api/v1",

"repos": {
    "user/repo": {
        "authorization": "token 0000000000000000000000000000000000000000",
        "secret": "a long passphrase",
        "project": "foo",
        "name": "prepare",
        "cmd": "cihelper git https://gitea.example/user/repo",
        "env": "FOO=bar",
        "tag": "native,linux"
    }
}
```

* Set `apiBaseUrl` so that Aura can reach the API of your Gitea server (it needs to end in `/api/v1`)
* `user/repo` is the username plus reponame pair of the repository on Gitea
* `authorization` contains an access token with "write:repository" permissions from a user that can write to this repository (created in your user settings under Applications); the token itself is prefixed with the string `token `
* `secret` is a long passphrase that is configured for the webhook
* `project` is the project slug on Aura
* `name`, `cmd`, `env`, `tag` are the same as in the SubmitRequest

## Setup

* In your repository go to **Settings** > **Webhooks** and add a "Gitea" webhook
* Set the target URL to this integrations endpoint
* Select "POST" as method, "application/json" as content type, and triggering on push events
* Put the same long passphrase into the **Secret** box as in the config