# AI assistants (MCP)

FodyCloud runs a [Model Context Protocol](https://modelcontextprotocol.io) server, so an AI
assistant such as Claude Code can create, deploy and debug your apps when you ask it to. This
page shows how to connect one and what it can do.

**You need:** an MCP client that supports remote servers over Streamable HTTP, and an API token
(see [Authentication](authentication.md)).

## Connect your assistant

| Setting | Value |
|---|---|
| URL | `https://mcp.fodycloud.example/mcp` |
| Transport | Streamable HTTP |
| Header | `Authorization: Bearer fc_…` |

**Claude Code:**

```bash
claude mcp add --transport http fodycloud https://mcp.fodycloud.example/mcp \
  --header "Authorization: Bearer fc_…"
```

**Other clients** that use a JSON configuration file:

```json
{
  "mcpServers": {
    "fodycloud": {
      "type": "http",
      "url": "https://mcp.fodycloud.example/mcp",
      "headers": { "Authorization": "Bearer fc_…" }
    }
  }
}
```

Then ask, for example: *"Deploy https://github.com/you/my-api on FodyCloud as my-api."*

**Choose the token's scope deliberately.** With a read-only token (**Allow changes**
unticked), the assistant can look at your apps and logs but can't change anything. That is a
good default for debugging.

## How an assistant deploys a new app

The tools follow the same review step as the dashboard, so you stay in control of what is
deployed:

1. `create_app` without a language: FodyCloud analyses the repository. Nothing is deployed.
2. `get_app` until the status is `review`. The assistant shows you the detected stack.
3. You confirm or correct it.
4. `confirm_stack` with your choice: FodyCloud builds and deploys.
5. `get_deployment_status` until `running` or `failed`; on failure, `get_build_logs` to find
   out why.

The assistant passes a language to `create_app` directly only when you have already told it
which stack to use.

## Tools

| Tool | Changes things | What it does |
|---|---|---|
| `list_stacks` | no | Languages, frameworks with default commands, and versions. |
| `list_apps` | no | Your apps with their status and URL. |
| `get_app` | no | One app, with its stack and the detection to review. |
| `create_app` | yes | Creates an app. Arguments: `name`, `repo_url`, `branch`, `env`, and optionally `language`, `framework`, `version`, `build_command`, `start_command`, `publish_dir`, `root_dir`, `health_check_path`. |
| `confirm_stack` | yes | Confirms or changes the stack, then builds. Arguments: `app`, `language`, `framework`, and optionally `version`, `build_command`, `start_command`, `publish_dir`, `root_dir`. |
| `analyse_repository` | yes | Runs the detection again, optionally in `root_dir`. |
| `deploy` | yes | Builds and deploys the latest commit. |
| `redeploy` | yes | Restarts the current version without rebuilding. |
| `rollback` | yes | Goes back to the previous working version. |
| `get_deployment_status` | no | Status of the latest or a given deployment. |
| `get_build_logs` | no | The build log of a deployment. |
| `set_health_check` | yes | Sets the path checked before each new deployment goes live (empty turns it off). |
| `get_runtime_logs` | no | What the running app printed. |
| `get_app_metrics` | no | CPU, memory and requests over 24 h, 7 days or 30 days, with the plan's limits. |
| `list_env_vars` | no | Variable names, with masked values. |
| `set_env_vars` | yes | Sets or removes variables; restarts the app. |
| `add_custom_domain` | yes | Attaches a domain and returns the DNS record to create. |
| `get_domain_status` | no | Checks a domain's DNS. |
| `remove_custom_domain` | yes, destructive | Detaches a domain from an app. The assistant must repeat the hostname as confirmation. |
| `list_services` | no | The app's volume, database and cache, with usage, limits and connection URLs. |
| `add_service` | yes | Adds a `volume`, a `postgres` database or a `valkey` cache, if the plan includes it. |
| `remove_service` | yes, destructive | Deletes a volume, database or cache with its data. The assistant must repeat the app name as confirmation. |
| `get_plan_and_usage` | no | Your plan, limits and number of apps. |
| `stop_app`, `start_app` | yes | Stops or starts an app. |
| `delete_app` | yes, destructive | Deletes an app. The assistant must repeat the app's name as confirmation. |

Read-only tools are marked as such, and `delete_app` is marked destructive, so clients that ask
for approval before risky actions can do so.

## Safety

- **The rules are the same as in the dashboard:** ownership, plan limits, email verification and
  rate limits (120 requests per minute, shared with the API).
- **Admin actions are never exposed** over MCP.
- **Logs are untrusted text.** Build and runtime logs contain whatever your code and its
  visitors print. A log could contain text written to mislead an assistant; the tool
  descriptions tell assistants to treat it as data, not instructions. Review what an assistant
  proposes to do after reading logs.
- **Revoke the token** from **Account & API** to cut an assistant's access at once.

## Related

- [API reference](api-reference.md): the same operations over HTTP.
- [Authentication](authentication.md)
