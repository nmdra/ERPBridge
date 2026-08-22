## bridgectl token create

Create an API token

### Synopsis

The raw token is printed only in the create response. Save it in a secret
manager before the command output is lost.

```
bridgectl token create --name <name> [flags]
```

### Options

```
      --expires string    Expiry as RFC3339 or a positive duration
  -h, --help              help for create
      --name string       Token display name
      --role stringArray  Token role (repeatable)
      --scope stringArray Token scope (repeatable: mcp, metrics, logs)
```

### Options inherited from parent commands

```
  -c, --context string   Override active context
  -o, --output string    Output format: table, json, yaml (default "table")
      --token string     API token for the ERPBridge server
  -v, --verbose          Show full HTTP request/response detail
```

### Examples

```
bridgectl --token "$API_AUTH_TOKEN" token create --name finance-agent --scope mcp --role finance_reader --expires 720h
```
