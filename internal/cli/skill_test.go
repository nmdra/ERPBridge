package cli

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	bridgectlops "github.com/nmdra/ERPBridge/skills/bridgectl-ops"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/nmdra/ERPBridge/internal/testutil"
)

func TestSkillEmbeddedFiles(t *testing.T) {
	files := embeddedSkillFiles(t)
	require.Contains(t, files, "SKILL.md")
	require.Contains(t, files, "references/ecosystem.md")
	require.Contains(t, files, "references/mcp.md")
	require.Contains(t, files, "references/preflight.md")
	require.Contains(t, files, "references/knowledge.md")
	require.Contains(t, files, "assets/plugin.yaml")
	require.Contains(t, files, "assets/execution-record.json")
	require.Contains(t, files, "assets/knowledge-pattern.md")
	require.Contains(t, files, "assets/skill-proposal.md")
	require.NotContains(t, files, "evals/evals.json")
	require.NotContains(t, files, ".agents/skill-memory/bridgectl-ops/index.md")
}

func TestSkillFrontmatterAndSections(t *testing.T) {
	data, err := bridgectlops.Files.ReadFile("SKILL.md")
	require.NoError(t, err)
	parts := strings.SplitN(string(data), "---\n", 3)
	require.Len(t, parts, 3, "skill must have YAML frontmatter")
	var frontmatter struct {
		Name          string            `yaml:"name"`
		Description   string            `yaml:"description"`
		Compatibility string            `yaml:"compatibility"`
		Metadata      map[string]string `yaml:"metadata"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &frontmatter))
	require.Equal(t, "bridgectl-ops", frontmatter.Name)
	require.NotEmpty(t, frontmatter.Description)
	require.LessOrEqual(t, utf8.RuneCountInString(frontmatter.Description), 1024)
	require.LessOrEqual(t, utf8.RuneCountInString(frontmatter.Compatibility), 500)
	require.NotEmpty(t, frontmatter.Metadata["version"])
	require.Less(t, len(strings.Split(parts[2], "\n")), 500)
	for _, section := range strings.Split(parts[2], "\n## ")[1:] {
		heading, body, found := strings.Cut(section, "\n")
		require.True(t, found, "section %q must have a body", heading)
		require.NotEmpty(t, strings.TrimSpace(body), "section %q must not be empty", heading)
	}
}

func TestSkillInstalledLinksStayWithinBundle(t *testing.T) {
	links := regexp.MustCompile(`\[[^\]\n]*\]\(([^\s)]+)\)`)
	for _, layout := range []string{"global/.agents/skills", "project/.agents/skills"} {
		t.Run(layout, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), filepath.FromSlash(layout), bridgectlOpsSkillName)
			require.NoError(t, installEmbeddedSkill(destination, bridgectlops.Files, false))
			for name, data := range embeddedSkillFiles(t) {
				if filepath.Ext(name) != ".md" {
					continue
				}
				for _, match := range links.FindAllStringSubmatch(string(data), -1) {
					target, err := url.Parse(match[1])
					require.NoError(t, err, "%s: %s", name, match[1])
					if target.IsAbs() || target.Host != "" || target.Path == "" {
						continue
					}
					require.False(t, filepath.IsAbs(target.Path), "%s: absolute local link", name)
					resolved := filepath.Join(destination, filepath.Dir(filepath.FromSlash(name)), filepath.FromSlash(target.Path))
					relative, err := filepath.Rel(destination, resolved)
					require.NoError(t, err)
					require.NotEqual(t, "..", relative, "%s: link escapes bundle", name)
					require.False(t, strings.HasPrefix(relative, ".."+string(filepath.Separator)), "%s: link escapes bundle: %s", name, match[1])
					require.FileExists(t, resolved, "%s: unresolved link: %s", name, match[1])
				}
			}
		})
	}
}

func TestResolveSkillDestination(t *testing.T) {
	home, project, custom := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "custom-skill")
	tests := []struct {
		name       string
		project    bool
		explicit   string
		expected   string
		wantErrMsg string
	}{
		{
			name:     "global default",
			expected: filepath.Join(home, ".agents", "skills", bridgectlOpsSkillName),
		},
		{
			name:     "project default",
			project:  true,
			expected: filepath.Join(project, ".agents", "skills", bridgectlOpsSkillName),
		},
		{
			name:     "explicit directory",
			explicit: custom,
			expected: custom,
		},
		{
			name:       "conflicting selectors",
			project:    true,
			explicit:   custom,
			wantErrMsg: "--dir cannot be used with --project",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveSkillDestination(test.project, test.explicit, home, project)
			if test.wantErrMsg != "" {
				require.EqualError(t, err, test.wantErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.expected, got)
		})
	}
}

func TestInstallEmbeddedSkillWritesCompleteTree(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "nested", bridgectlOpsSkillName)
	require.NoError(t, installEmbeddedSkill(destination, bridgectlops.Files, false))

	for path, expected := range embeddedSkillFiles(t) {
		// #nosec G304 -- path comes from the trusted embedded filesystem and test temp directory.
		actual, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(path)))
		require.NoError(t, err, path)
		require.Equal(t, expected, actual, path)
	}
	_, err := os.Stat(filepath.Join(destination, "evals"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestInstallEmbeddedSkillRequiresForceAndReplacesTree(t *testing.T) {
	destination := filepath.Join(t.TempDir(), bridgectlOpsSkillName)
	require.NoError(t, installEmbeddedSkill(destination, bridgectlops.Files, false))

	sentinel := filepath.Join(destination, "local-change.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("local"), 0600))
	err := installEmbeddedSkill(destination, bridgectlops.Files, false)
	require.EqualError(t, err, fmt.Sprintf("skill destination %q already exists; use --force to replace it", destination))
	require.FileExists(t, sentinel)

	require.NoError(t, installEmbeddedSkill(destination, bridgectlops.Files, true))
	_, err = os.Stat(sentinel)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestInstallEmbeddedSkillRejectsDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks may require elevated permissions on Windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, bridgectlOpsSkillName)
	require.NoError(t, os.Mkdir(target, 0750))
	require.NoError(t, os.Symlink(target, link))

	err := installEmbeddedSkill(link, bridgectlops.Files, true)
	require.EqualError(t, err, fmt.Sprintf("refusing to install through symlink %q", link))
}

func TestSkillInstallCommandUsesExplicitDirectory(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Cleanup(resetSkillInstallCommand)
	resetSkillInstallCommand()

	destination := filepath.Join(t.TempDir(), bridgectlOpsSkillName)
	var output bytes.Buffer
	skillInstallCmd.SetOut(&output)
	require.NoError(t, skillInstallCmd.Flags().Set("dir", destination))
	require.NoError(t, runSkillInstall(skillInstallCmd, nil))
	require.Contains(t, output.String(), "installed bridgectl-ops skill to "+destination)
	require.FileExists(t, filepath.Join(destination, "SKILL.md"))
}

func TestSkillInstallCommandRejectsConflictingSelectors(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Cleanup(resetSkillInstallCommand)
	resetSkillInstallCommand()
	require.NoError(t, skillInstallCmd.Flags().Set("project", "true"))
	require.NoError(t, skillInstallCmd.Flags().Set("dir", filepath.Join(t.TempDir(), bridgectlOpsSkillName)))
	err := runSkillInstall(skillInstallCmd, nil)
	require.EqualError(t, err, "--dir cannot be used with --project")
}

func embeddedSkillFiles(t *testing.T) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte)
	require.NoError(t, fs.WalkDir(bridgectlops.Files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(bridgectlops.Files, path)
		if err != nil {
			return err
		}
		files[path] = data
		return nil
	}))
	return files
}

func resetSkillInstallCommand() {
	_ = skillInstallCmd.Flags().Set("project", "false")
	_ = skillInstallCmd.Flags().Set("dir", "")
	_ = skillInstallCmd.Flags().Set("force", "false")
	skillInstallCmd.SetOut(io.Discard)
}

var _ fs.FS = bridgectlops.Files
