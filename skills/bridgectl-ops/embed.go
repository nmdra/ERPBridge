// Package bridgectlops contains the distributable ERPBridge operations skill.
package bridgectlops

import "embed"

// Files contains the skill instructions and supporting reference assets.
//
// Evaluation files are intentionally excluded from the distributable skill.
//
//go:embed SKILL.md references assets
var Files embed.FS
