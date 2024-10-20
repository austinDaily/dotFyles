package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "initializes program",
	Long:  "creates new dir called dotFyles, collects your important dotfiles and copies them into dotFyles dir with symlinks, initilizes git repo, and pushes repo to your Github account.",

	Run: createDotfyles,
}

func createDotfyles(cmd *cobra.Command, args []string) {
	// get users home dir
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error retrieving Home directory:", err)
		return
	}
	// create the dotFyles dir path
	dotFylesDir := filepath.Join(homeDir, "dotFyles")
	//create the dotFyles directory
	err = os.MkdirAll(dotFylesDir, 0755) // 0755 gives rwx permissions
	if err != nil {
		fmt.Println("Error creating dotFyles directory:", err)
		return
	}
	fmt.Println("dotFyles directory successfully created at:", dotFylesDir)
	// initialize git repo
	initializeRepo(dotFylesDir)
	// find and copy/symlink the config files
	findConfigs(dotFylesDir)
}

func initializeRepo(newPath string) {
	_, err := git.PlainInit(newPath, false) // false for non-bare repo
	if err != nil {
		fmt.Println("Error initializing git repo", err)
		return
	}
	fmt.Println("Successfully initialized git repo")
}

type Config struct {
	Path        string
	IsDirectory bool
}

var configs = []Config{
	{Path: ".bashrc", IsDirectory: false},
	{Path: ".bash_profile", IsDirectory: false},
	{Path: ".zshrc", IsDirectory: false},
	{Path: ".profile", IsDirectory: false},
	{Path: ".fish/config.fish", IsDirectory: false},
	{Path: ".config/fish/", IsDirectory: true},
	{Path: ".vimrc", IsDirectory: false},
	{Path: ".config/nvim/", IsDirectory: true},
	{Path: ".emacs.d/init.el", IsDirectory: false},
	{Path: ".config/helix/", IsDirectory: true},
	{Path: ".tmux.conf", IsDirectory: false},
	{Path: ".zellij/", IsDirectory: true},
	{Path: ".config/wezterm/", IsDirectory: true},
	{Path: ".wezterm.lua", IsDirectory: false},
	{Path: ".config/alacritty/", IsDirectory: true},
	{Path: ".alacritty.yml", IsDirectory: false},
	{Path: ".config/kitty/", IsDirectory: true},
	{Path: ".config/starship.toml", IsDirectory: false},
	{Path: ".config/i3/", IsDirectory: true},
	{Path: ".config/sway/", IsDirectory: true},
	{Path: ".config/hypr/", IsDirectory: true},
	{Path: ".config/xfce4/", IsDirectory: true},
	{Path: ".gitconfig", IsDirectory: false},
	{Path: ".gitignore_global", IsDirectory: false},
	{Path: ".config/ranger/", IsDirectory: true},
	{Path: ".config/picom.conf", IsDirectory: false},
	{Path: ".config/dunst/", IsDirectory: true},
	{Path: ".config/rofi", IsDirectory: true},
	{Path: ".config/swaylock", IsDirectory: true},
	{Path: ".ssh/config/", IsDirectory: true},
	{Path: ".config/gtk-3.0/", IsDirectory: true},
}

func findConfigs(dotFylesDir string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error retrieving Home directory:", err)
		return
	}

	for _, config := range configs {
		fullPath := filepath.Join(homeDir, config.Path)
		destPath := filepath.Join(dotFylesDir, filepath.Base(config.Path))

		//check if file or directory exists
		info, err := os.Stat(fullPath)
		if err != nil {
			if info.IsDir() {
				// its a directory, create a symlink
				err = symLinkDirectory(fullPath, destPath)
				if err != nil {
					fmt.Println("Error symlinking directory:", err)
				}
			} else {
				// its a file, copy it
				err = copyFile(fullPath, destPath)
				if err != nil {
					fmt.Println("Error copying file:", err)
				}
			}
		} else {
			fmt.Println("Not found:", fullPath)
		}
	}
}

func copyFile(src, dst string) error {
	// open the source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening source file: %w", err)
	}
	defer sourceFile.Close()

	// create the destination (copy) file in dotFyles
	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("error creating destination file: %w", err)
	}
	defer destFile.Close()

	// copy the contents from the sourceFile to destFile
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("error copying file: %w", err)
	}

	// copy the permissions from sourceFile to destFile
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("error retrieving sourceFile info: %w", err)
	}

	err = os.Chmod(dst, srcInfo.Mode())
	if err != nil {
		return fmt.Errorf("error setting file permissions: %w", err)
	}

	fmt.Println("Copied file:", src, "to", dst)
	return nil
}

func symLinkDirectory(src, dst string) error {
	// check if symlink already exists
	if _, err := os.Lstat(dst); err == nil {
		fmt.Println("Symlink already exists:", dst)
		return nil
	}
	//create the symbolic link
	err := os.Symlink(src, dst)
	if err != nil {
		return fmt.Errorf("error creating symlink: %w", err)
	}
	fmt.Println("Created symlink from:", src, "to", dst)
	return nil
}

func addAndCommit(repoDir string) {
	// open the git repo in the dotFyles directory
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		fmt.Println("Error opening git repo:", err)
		return
	}
	// get the working directory for the repo
	worktree, err := repo.Worktree()
	if err != nil {
		fmt.Println("Error retrieving worktree:", err)
		return
	}
	// git add
	err = worktree.AddGlob(".")
	if err != nil {
		fmt.Println("Error adding files to staging area:", err)
		return
	}
	fmt.Println("Staged all files in dotFyles directory")
	// git commit
	usersGitName := ""  //make sure to prompt user for this info
	usersGitEmail := "" //make sure to prompt user for this info
	commitMessage := "Initial commit"
	commit, err := worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  usersGitName,  // You can customize this
			Email: usersGitEmail, // Customize this too
			When:  time.Now(),
		},
	})
	if err != nil {
		fmt.Println("Error commiting files:", err)
		return
	}

}
