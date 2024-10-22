package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	transport "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "initializes program",
	Long:  "creates new dir called dotfyles, collects your important dotfiles and copies them into dotfyles dir with symlinks, initilizes git repo, and pushes repo to your Github account.",

	Run: createDotfyles,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func createDotfyles(cmd *cobra.Command, args []string) {
	// Authenticate with GitHub
	accessToken, err := authenticateWithGitHub()
	if err != nil {
		fmt.Println("Error during GitHub authentication:", err)
		return
	}
	// accessToken for further GitHub API calls if needed
	fmt.Println("GitHub authentication successful. Access token:", accessToken)
	// get users home dir
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error retrieving Home directory:", err)
		return
	}
	// create the dotfyles dir path
	dotfylesDir := filepath.Join(homeDir, "dotfyles")
	//create the dotfyles directory
	err = os.MkdirAll(dotfylesDir, 0755) // 0755 gives rwx permissions
	if err != nil {
		fmt.Println("Error creating dotfyles directory:", err)
		return
	}
	fmt.Println("dotfyles directory successfully created at:", dotfylesDir)
	// initialize git repo
	initializeRepo(dotfylesDir)
	// find and copy/symlink the config files
	findConfigs(dotfylesDir)
	// stage and commit newly added config files
	addAndCommit(dotfylesDir)
	// push the repo to github
	err = pushToGitHub(dotfylesDir, accessToken)
	if err != nil {
		fmt.Println("Error pushing to GitHub:", err)
	}
}

func authenticateWithGitHub() (string, error) {
	// request device and user verification codes
	deviceAuthURL := "https://github.com/login/device/code"
	clientID := "Ov23liNHy37PEdYFK4Jf"

	deviceAuthRequest := map[string]interface{}{
		"client_id": clientID,
		"scope":     "repo", // adjust scopes as needed
	}

	reqBody, _ := json.Marshal(deviceAuthRequest)
	resp, err := http.Post(deviceAuthURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to request device code: %s", resp.Status)
	}

	var deviceAuthResponse struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&deviceAuthResponse); err != nil {
		return "", err
	}

	// prompt user to enter user code
	fmt.Printf("Please go to %s and enter the code: %s\n", deviceAuthResponse.VerificationURI, deviceAuthResponse.UserCode)

	// poll for authorization status
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
	}

	for {
		time.Sleep(time.Duration(deviceAuthResponse.Interval) * time.Second)

		tokenReq, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", nil)
		if err != nil {
			return "", err
		}

		q := tokenReq.URL.Query()
		q.Add("client_id", clientID)
		q.Add("device_code", deviceAuthResponse.DeviceCode)
		q.Add("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		tokenReq.URL.RawQuery = q.Encode()
		tokenReq.Header.Set("Accept", "application/json")

		// set auth header for using client ID and no secret
		tokenReq.SetBasicAuth(clientID, "")

		resp, err := http.DefaultClient.Do(tokenReq)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
				return "", err
			}
			return tokenResponse.AccessToken, nil
		} else if resp.StatusCode == http.StatusForbidden {
			fmt.Println("Authorization pending... Please check your browser.")
		} else {
			return "", fmt.Errorf("failed to obtain access token: %s", resp.Status)
		}
	}
}

func pushToGitHub(repoDir string, accessToken string) error {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("error opening git repo: %w", err)
	}

	// create a new remote pointing to the GitHub repo
	remoteURL := "https://github.com/YOUR_GITHUB_USERNAME/dotfyles.git" // update this to direct to users github
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		return fmt.Errorf("error creating remote: %w", err)
	}

	// push to GitHub using the acess token for auth
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth: &transport.BasicAuth{
			Username: "oauth2", // this is required
			Password: accessToken,
		},
	})
	if err != nil {
		return fmt.Errorf("error pushing to GitHub: %w", err)
	}

	fmt.Println("Successfully pushed to GitHub repository!")
	return nil
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

func findConfigs(dotfylesDir string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Error retrieving Home directory:", err)
		return
	}

	for _, config := range configs {
		fullPath := filepath.Join(homeDir, config.Path)
		destPath := filepath.Join(dotfylesDir, filepath.Base(config.Path))

		//check if file or directory exists
		info, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("Not found:", fullPath)
				continue
			} else {
				fmt.Println("Error checking path:", fullPath)
				continue
			}
		}
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
	}
}

func copyFile(src, dst string) error {
	// open the source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("error opening source file: %w", err)
	}
	defer sourceFile.Close()

	// create the destination (copy) file in dotfyles
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
	// open the git repo in the dotfyles directory
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
	fmt.Println("Staged all files in dotfyles directory")
	// git commit
	//usersGitName := ""  //make sure to prompt user for this info
	//usersGitEmail := "" //make sure to prompt user for this info
	commitMessage := "Initial commit"
	commit, err := worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "austinDaily",             // You can customize this
			Email: "snowtheparrot@proton.me", // Customize this too
			When:  time.Now(),
		},
	})
	if err != nil {
		fmt.Println("Error committing files:", err)
		return
	}
	// print the commit hash
	obj, err := repo.CommitObject(commit)
	if err != nil {
		fmt.Println("Error retrieving commit object:", err)
		return
	}
	fmt.Println("Committed files with commit hash:", obj.Hash)

}
