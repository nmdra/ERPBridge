package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	bridgectlops "github.com/nmdra/ERPBridge/skills/bridgectl-ops"
	"github.com/spf13/cobra"
)

const bridgectlOpsSkillName = "bridgectl-ops"

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage bundled Agent Skills",
	Long: `Manage Agent Skills bundled with bridgectl.

The bundled bridgectl-ops skill contains safe ERPBridge operating guidance,
references, and manifest templates. Installation is local and does not contact
an ERPBridge server.`,
}

var skillInstallCmd = &cobra.Command{
	Use:     "install",
	Short:   "Install the bundled bridgectl-ops skill",
	Args:    cobra.NoArgs,
	Example: "  bridgectl skill install\n  bridgectl skill install --project\n  bridgectl skill install --dir /path/to/bridgectl-ops --force",
	RunE:    runSkillInstall,
}

func runSkillInstall(cmd *cobra.Command, _ []string) error {
	project, err := cmd.Flags().GetBool("project")
	if err != nil {
		return fmt.Errorf("read project flag: %w", err)
	}
	explicitDir, err := cmd.Flags().GetString("dir")
	if err != nil {
		return fmt.Errorf("read dir flag: %w", err)
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return fmt.Errorf("read force flag: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	destination, err := resolveSkillDestination(project, explicitDir, homeDir, workingDir)
	if err != nil {
		return err
	}
	if err := installEmbeddedSkill(destination, bridgectlops.Files, force); err != nil {
		return fmt.Errorf("install %s skill: %w", bridgectlOpsSkillName, err)
	}

	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "installed %s skill to %s\n", bridgectlOpsSkillName, destination); err != nil {
		return fmt.Errorf("write install result: %w", err)
	}
	return nil
}

func resolveSkillDestination(project bool, explicitDir, homeDir, workingDir string) (string, error) {
	if project && explicitDir != "" {
		return "", fmt.Errorf("--dir cannot be used with --project")
	}
	if explicitDir != "" {
		destination, err := filepath.Abs(explicitDir)
		if err != nil {
			return "", fmt.Errorf("resolve skill directory %q: %w", explicitDir, err)
		}
		return filepath.Clean(destination), nil
	}

	baseDir := homeDir
	if project {
		baseDir = workingDir
	}
	if baseDir == "" {
		if project {
			return "", fmt.Errorf("resolve project skill directory: working directory is empty")
		}
		return "", fmt.Errorf("resolve global skill directory: home directory is empty")
	}
	return filepath.Join(baseDir, ".agents", "skills", bridgectlOpsSkillName), nil
}

func installEmbeddedSkill(destination string, files fs.FS, force bool) error {
	destination = filepath.Clean(destination)
	existing, err := os.Lstat(destination)
	switch {
	case err == nil:
		if existing.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to install through symlink %q", destination)
		}
		if !existing.IsDir() {
			return fmt.Errorf("skill destination %q is not a directory", destination)
		}
		if !force {
			return fmt.Errorf("skill destination %q already exists; use --force to replace it", destination)
		}
	case !os.IsNotExist(err):
		return fmt.Errorf("inspect skill destination %q: %w", destination, err)
	}

	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0750); err != nil {
		return fmt.Errorf("create skill parent directory: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(destination)+".tmp-")
	if err != nil {
		return fmt.Errorf("create staged skill directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	if err := materializeSkillFiles(stage, files); err != nil {
		return err
	}

	if existing == nil {
		if err := os.Rename(stage, destination); err != nil {
			return fmt.Errorf("publish skill directory: %w", err)
		}
		return nil
	}

	backup, err := os.MkdirTemp(parent, "."+filepath.Base(destination)+".backup-")
	if err != nil {
		return fmt.Errorf("create skill backup directory: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare skill backup directory: %w", err)
	}
	if err := os.Rename(destination, backup); err != nil {
		return fmt.Errorf("stage existing skill directory: %w", err)
	}
	if err := os.Rename(stage, destination); err != nil {
		_ = os.Rename(backup, destination)
		return fmt.Errorf("publish replacement skill directory: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove previous skill directory: %w", err)
	}
	return nil
}

func materializeSkillFiles(destination string, files fs.FS) error {
	fileCount := 0
	err := fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return nil
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, 0750)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("embedded skill file %q is a symlink", path)
		}

		data, err := fs.ReadFile(files, path)
		if err != nil {
			return fmt.Errorf("read embedded skill file %q: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
			return fmt.Errorf("create directory for embedded skill file %q: %w", path, err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return fmt.Errorf("write embedded skill file %q: %w", path, err)
		}
		fileCount++
		return nil
	})
	if err != nil {
		return fmt.Errorf("materialize embedded skill: %w", err)
	}
	if fileCount == 0 {
		return fmt.Errorf("materialize embedded skill: no files found")
	}
	return nil
}

func init() {
	RootCmd.AddCommand(skillCmd)
	skillCmd.AddCommand(skillInstallCmd)
	skillInstallCmd.Flags().Bool("project", false, "Install into ./.agents/skills in the current project")
	skillInstallCmd.Flags().String("dir", "", "Install directly into this skill directory")
	skillInstallCmd.Flags().Bool("force", false, "Replace an existing skill directory")
}
