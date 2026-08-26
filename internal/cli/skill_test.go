package cli

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	bridgectlops "github.com/nmdra/ERPBridge/skills/bridgectl-ops"
	"github.com/stretchr/testify/require"
)

func TestSkillEmbeddedFiles(t *testing.T) {
	files := embeddedSkillFiles(t)
	require.Contains(t, files, "SKILL.md")
	require.Contains(t, files, "references/ecosystem.md")
	require.Contains(t, files, "assets/plugin.yaml")
	require.NotContains(t, files, "evals/evals.json")
}

func TestResolveSkillDestination(t *testing.T) {
	tests := []struct {
		name       string
		project    bool
		explicit   string
		expected   string
		wantErrMsg string
	}{
		{
			name:     "global default",
			expected: filepath.Join("/home/tester", ".agents", "skills", bridgectlOpsSkillName),
		},
		{
			name:     "project default",
			project:  true,
			expected: filepath.Join("/work/project", ".agents", "skills", bridgectlOpsSkillName),
		},
		{
			name:     "explicit directory",
			explicit: "/tmp/custom-skill",
			expected: "/tmp/custom-skill",
		},
		{
			name:       "conflicting selectors",
			project:    true,
			explicit:   "/tmp/custom-skill",
			wantErrMsg: "--dir cannot be used with --project",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveSkillDestination(test.project, test.explicit, "/home/tester", "/work/project")
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
	require.EqualError(t, err, "skill destination "+`"`+destination+`"`+" already exists; use --force to replace it")
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
	require.EqualError(t, err, "refusing to install through symlink "+`"`+link+`"`)
}

func TestSkillInstallCommandUsesExplicitDirectory(t *testing.T) {
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
