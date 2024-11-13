package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	// Request device and user verification codes
	deviceAuthURL := "https://github.com/login/device/code"
	clientID := "Ov23liNHy37PEdYFK4Jf"

	deviceAuthRequest := map[string]string{
		"client_id": clientID,
		"scope":     "repo",
	}

	reqBody, _ := json.Marshal(deviceAuthRequest)
	resp, err := http.Post(deviceAuthURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("error making POST request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	values, err := url.ParseQuery(string(body))
	if err != nil {
		return "", fmt.Errorf("error parsing response body: %w", err)
	}

	deviceCode := values.Get("device_code")
	userCode := values.Get("user_code")
	verificationURI := values.Get("verification_uri")
	interval, err := strconv.Atoi(values.Get("interval"))
	if err != nil {
		return "", fmt.Errorf("error parsing interval: %w", err)
	}

	fmt.Printf("Please go to %s and enter the code: %s\n", verificationURI, userCode)
	fmt.Scan() // Wait for user input before polling

	tokenURL := "https://github.com/login/oauth/access_token"

	// Poll for access token until user authorizes or an error occurs
	for {
		fmt.Println("Polling GitHub for access token...")

		// Wait the specified interval before making the request
		time.Sleep(time.Duration(interval) * time.Second)

		tokenRequestBody := url.Values{}
		tokenRequestBody.Set("client_id", clientID)
		tokenRequestBody.Set("device_code", deviceCode)
		tokenRequestBody.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

		req, err := http.NewRequest("POST", tokenURL, bytes.NewBufferString(tokenRequestBody.Encode()))
		if err != nil {
			return "", err
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("error reading response body: %w", err)
		}

		// Print debugging information
		//fmt.Printf("Response Status: %s\n", resp.Status)
		//fmt.Printf("Response Headers: %v\n", resp.Header)
		//fmt.Printf("Raw response body from GitHub: %s\n", string(body))

		if resp.StatusCode == http.StatusOK {
			var tokenResponse struct {
				AccessToken string `json:"access_token"`
			}

			err = json.Unmarshal(body, &tokenResponse)
			if err != nil {
				return "", fmt.Errorf("error decoding token response: %w", err)
			}

			if tokenResponse.AccessToken == "" {
				fmt.Println("No response yet...")
				continue
			}

			fmt.Println("GitHub authentication successful.")
			return tokenResponse.AccessToken, nil

		} else if resp.StatusCode == 428 { // "authorization_pending"
			fmt.Println("Authorization still pending... retrying in", interval, "seconds.")

		} else if resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("authorization failed; user may have denied the request")

		} else {
			return "", fmt.Errorf("failed to obtain access token: %s - %s", resp.Status, string(body))
		}
	}
}

func pushToGitHub(repoDir string, accessToken string) error {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("error opening git repo: %w", err)
	}

	remoteURL := "https://github.com/austinDaily/dotfyles.git" // Update to direct to the user's actual GitHub repo

	// Check if the remote "origin" exists; create it if it doesn’t
	_, err = repo.Remote("origin")
	if err == git.ErrRemoteNotFound {
		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{remoteURL},
		})
		if err != nil {
			return fmt.Errorf("error creating remote: %w", err)
		}
		fmt.Println("Remote 'origin' created successfully.")
	} else if err != nil {
		return fmt.Errorf("error checking remote: %w", err)
	} else {
		fmt.Println("Remote 'origin' already exists. Reusing existing remote.")
	}

	// Fetch the latest changes from the remote repository
	err = repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Auth: &transport.BasicAuth{
			Username: "oauth2",
			Password: accessToken,
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("error fetching from GitHub: %w", err)
	}

	// Get the worktree for checking and staging changes
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("error retrieving worktree: %w", err)
	}

	// Check for local changes to avoid unnecessary push attempts
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("error checking worktree status: %w", err)
	}

	if status.IsClean() {
		fmt.Println("No changes to push; repository is up-to-date.")
		return nil
	}

	// Push to github
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth: &transport.BasicAuth{
			Username: "oauth2",
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

// Prompt for user input in the terminal
func promptUserInput(promptText string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(promptText)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
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

	// Prompt user for Git username and email
	username := promptUserInput("Enter your Git username: ")
	email := promptUserInput("Enter your Git email: ")

	// git commit
	commitMessage := "Initial commit"
	commit, err := worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  username,
			Email: email,
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
