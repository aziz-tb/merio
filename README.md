<a href="https://zerodha.tech"><img src="https://zerodha.tech/static/images/github-badge.svg" align="right" alt="Zerodha Tech Badge" /></a>


# merio

Modern, open source, self-hosted omnichannel customer support desk. Live chat, email, and more in a single binary.

![image](https://merio.io/hero-dark-v2.png?q=2)


Visit [merio.io](https://merio.io) for more info. Check out the [**live demo**](https://demo.merio.io/).

## Features

- **Omnichannel inbox**  
  Live chat, email, and more — all in one inbox. Connect support@, billing@, sales@ and manage every conversation from a single, unified interface.
- **Live chat widget**  
  Embed a real-time chat widget on your website. Engage visitors instantly and handle live conversations right from your support desk.
- **Granular permissions**  
  Create custom roles with granular permissions for teams and individual agents.
- **Automations**  
  Eliminate repetitive tasks with powerful automation rules. Auto-tag, assign, and route conversations based on custom conditions.
- **CSAT surveys**  
  Measure customer satisfaction with automated surveys.
- **Macros**  
  Save frequently sent messages as templates. With one click, send saved responses, set tags, and more.
- **Organization**  
  Keep conversations organized with tags, custom statuses for conversations, and snoozing. Find any conversation instantly from the search bar.
- **Auto assignment**  
  Distribute workload with auto assignment rules. Auto-assign conversations based on agent capacity or custom criteria.
- **SLA management**  
  Set and track response time targets. Get notified when conversations are at risk of breaching SLA commitments.
- **Custom attributes**  
  Create custom attributes for contacts or conversations such as the subscription plan or the date of their first purchase. 
- **AI-assist**  
  Instantly rewrite responses with AI to make them more friendly, professional, or polished.
- **Activity logs**  
  Track all actions performed by agents and admins—updates and key events across the system—for auditing and accountability.
- **Webhooks**  
  Integrate with external systems using real-time HTTP notifications for conversation and message events.
- **Command bar**  
  Opens with a simple shortcut (CTRL+K) and lets you quickly perform actions on conversations.

And more — checkout [merio.io](https://merio.io) or try the [live demo](https://demo.merio.io/).


## Installation

### Docker

The latest image is available on DockerHub at [`merio/merio:latest`](https://hub.docker.com/r/merio/merio/tags?page=1&ordering=last_updated&name=latest)

```shell
# Download the compose file and sample config file in the current directory.
curl -LO https://github.com/aziz-tb/merio/raw/main/docker-compose.yml
curl -LO https://github.com/aziz-tb/merio/raw/main/config.sample.toml

# Copy the config.sample.toml to config.toml and edit it as needed.
cp config.sample.toml config.toml

# Configure the Secrets Manager KEK used for operator-managed function secrets.
export MERIO_SECRETS_KEK="$(openssl rand -base64 32)"

# Run the services in the background.
docker compose up -d

# Setting System user password.
docker exec -it merio_app ./merio --set-system-user-password
```

Go to `http://localhost:9000` and login with username `System` and the password you set using the `--set-system-user-password` command.

See [installation docs](https://docs.merio.io/getting-started/installation)

__________________

### Binary
- Download the [latest release](https://github.com/aziz-tb/merio/releases) and extract the merio binary.
- Edit config.toml as needed.
- Set `MERIO_SECRETS_KEK` to a base64-encoded 32-byte AES key (`openssl rand -base64 32`).
- `./merio --install` to setup the Postgres DB.
- Run `./merio --set-system-user-password` to set the password for the System user.
- Run `./merio` and visit `http://localhost:9000` and login with email `System` and the password you set using the --set-system-user-password command.

See [installation docs](https://docs.merio.io/getting-started/installation)
__________________

## Developers

- If you are interested in contributing, **please read [CONTRIBUTING.md](./CONTRIBUTING.md) first**.
- For local development and setup, refer to the [developer setup](https://docs.merio.io/contributing/developer-setup).
- For planned features and project direction, see [ROADMAP.md](./ROADMAP.md).

The backend is written in Go and the frontend is Vue.js 3 with Shadcn UI.



## Translators
You can help translate merio into your language on [Crowdin](https://crowdin.com/project/merio).
