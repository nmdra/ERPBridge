# <Concise failure title>

## Summary

<What failed, who is affected, and its operational impact.>

## Environment

- Context: `<context name>`
- Timestamp: `<RFC3339 with timezone>`
- Bridgectl: `<version>`
- ERPBridge server: `<version or commit>`
- SDK/client: `<name and version, if applicable>`

## Expected behavior

<Observable expected result.>

## Actual behavior

<Observable result and stable error code/message.>

## Minimal reproduction

1. <Safe setup step>
2. <Redacted command or MCP/SDK action>
3. <Observed result>

## Evidence

```text
<Redacted log excerpt, registry readback, or test output>
```

## Investigation performed

- <Relevant API/tool/schema version>
- <Checks, tests, and documentation consulted>
- <Likely component and confidence, if known>

## Security review

- Credentials, tokens, authorization headers, cookies, personal data, and ERP
  business records were removed or replaced with descriptive placeholders.
