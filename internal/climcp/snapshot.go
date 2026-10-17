package climcp

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// ChildSnapshotEnv carries the launch-time snapshot fingerprint to a child
// process. It is an internal consistency marker, never an authentication
// credential, and is never logged or returned to a client.
const ChildSnapshotEnv = "BRIDGECTL_MCP_CHILD_SNAPSHOT"

// pinnedChildEnvironment lists the child variables the adapter fixes for the
// lifetime of the MCP session, in deterministic order.
var pinnedChildEnvironment = []string{
	"BRIDGE_CONTEXT",
	"BRIDGE_SERVER",
	"BRIDGE_MCP_SERVER",
	"BRIDGE_API_TOKEN",
	ChildSnapshotEnv,
}

// Snapshot is the immutable launch-time selection for one MCP session.
// Changing the saved default context never changes an existing snapshot.
type Snapshot struct {
	ContextName string
	Server      string
	MCPServer   string
	APIToken    string
}

// Fingerprint returns the deterministic consistency fingerprint for this
// snapshot. It covers empty values as well, which environment overrides cannot
// pin, so an absent token cannot silently become a newly-configured one during
// the session.
func (s Snapshot) Fingerprint() string {
	digest := sha256.New()
	for _, part := range []string{s.ContextName, s.Server, s.MCPServer, s.APIToken} {
		// Length-prefix each part so different splits cannot collide.
		_, _ = digest.Write([]byte(strconv.Itoa(len(part))))
		_, _ = digest.Write([]byte(":"))
		_, _ = digest.Write([]byte(part))
		_, _ = digest.Write([]byte("\n"))
	}
	return hex.EncodeToString(digest.Sum(nil))
}

// ChildEnvironment returns base with every pinned variable removed, then the
// pinned session values and fingerprint appended. Inherited environment is
// otherwise preserved so the child keeps its normal CLI behavior.
func (s Snapshot) ChildEnvironment(base []string) []string {
	pinned := map[string]string{
		"BRIDGE_CONTEXT":    s.ContextName,
		"BRIDGE_SERVER":     s.Server,
		"BRIDGE_MCP_SERVER": s.MCPServer,
		"BRIDGE_API_TOKEN":  s.APIToken,
		ChildSnapshotEnv:    s.Fingerprint(),
	}
	result := make([]string, 0, len(base)+len(pinnedChildEnvironment))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			result = append(result, entry)
			continue
		}
		if _, isPinned := pinned[key]; isPinned {
			continue
		}
		result = append(result, entry)
	}
	for _, key := range pinnedChildEnvironment {
		result = append(result, key+"="+pinned[key])
	}
	return result
}
