package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// newStatusCommand creates the status command
func (app *App) newStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Get server status",
		Long:  "Display the current status of the TUF server including version, uptime, and metrics",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := app.getClient()
			status, err := client.GetStatus()
			if err != nil {
				return fmt.Errorf("failed to get status: %w", err)
			}
			return app.output(status)
		},
	}
	return cmd
}

// newConnectivityCommand creates the connectivity command
func (app *App) newConnectivityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connectivity",
		Short: "Check server connectivity",
		Long:  "Test connectivity to the TUF server and verify authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := app.getClient()
			result, err := client.CheckConnectivity()
			if err != nil {
				return fmt.Errorf("connectivity check failed: %w", err)
			}
			return app.output(result)
		},
	}
	return cmd
}

// newUploadCommand creates the upload command
func (app *App) newUploadCommand() *cobra.Command {
	var remotePath string
	var recursive bool

	cmd := &cobra.Command{
		Use:   "upload <file> [files...]",
		Short: "Upload files to the repository",
		Long:  "Upload one or more files to the TUF repository",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := app.getClient()
			var results []*UploadResult

			for _, localPath := range args {
				// Handle glob patterns
				matches, err := filepath.Glob(localPath)
				if err != nil {
					return fmt.Errorf("invalid pattern %s: %w", localPath, err)
				}

				if len(matches) == 0 {
					// Try as literal path
					matches = []string{localPath}
				}

				for _, match := range matches {
					info, err := os.Stat(match)
					if err != nil {
						results = append(results, &UploadResult{
							LocalPath: match,
							Success:   false,
							Error:     err.Error(),
						})
						continue
					}

					if info.IsDir() {
						if !recursive {
							results = append(results, &UploadResult{
								LocalPath: match,
								Success:   false,
								Error:     "is a directory (use -r for recursive)",
							})
							continue
						}
						// Handle recursive directory upload
						err := filepath.Walk(match, func(path string, info os.FileInfo, err error) error {
							if err != nil || info.IsDir() {
								return err
							}
							
							relPath, _ := filepath.Rel(match, path)
							targetPath := remotePath
							if targetPath == "" {
								targetPath = relPath
							} else {
								targetPath = filepath.Join(remotePath, relPath)
							}
							
							result, err := client.UploadFile(path, targetPath)
							if err != nil {
								result = &UploadResult{
									LocalPath: path,
									Success:   false,
									Error:     err.Error(),
								}
							}
							results = append(results, result)
							return nil
						})
						if err != nil {
							return fmt.Errorf("failed to walk directory %s: %w", match, err)
						}
					} else {
						// Single file upload
						targetPath := remotePath
						if targetPath == "" {
							targetPath = filepath.Base(match)
						}
						
						result, err := client.UploadFile(match, targetPath)
						if err != nil {
							result = &UploadResult{
								LocalPath: match,
								Success:   false,
								Error:     err.Error(),
							}
						}
						results = append(results, result)
					}
				}
			}

			// Output results
			if len(results) == 1 {
				return app.output(results[0])
			}
			return app.output(results)
		},
	}

	cmd.Flags().StringVarP(&remotePath, "path", "p", "", "Remote path in repository")
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "Upload directories recursively")

	return cmd
}

// newDeleteCommand creates the delete command
func (app *App) newDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <path> [paths...]",
		Short: "Delete files from the repository",
		Long:  "Delete one or more files from the TUF repository",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := app.getClient()
			var results []*DeleteResult

			for _, path := range args {
				result, err := client.DeleteFile(path)
				if err != nil {
					result = &DeleteResult{
						Path:    path,
						Success: false,
						Error:   err.Error(),
					}
				}
				results = append(results, result)
			}

			// Output results
			if len(results) == 1 {
				return app.output(results[0])
			}
			return app.output(results)
		},
	}
	return cmd
}

// newVerifyCommand creates the verify command
func (app *App) newVerifyCommand() *cobra.Command {
	var checkAll bool

	cmd := &cobra.Command{
		Use:   "verify [paths...]",
		Short: "Verify files in the repository",
		Long:  "Verify file integrity and metadata consistency in the TUF repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := app.getClient()
			var results []*VerifyResult

			paths := args
			if checkAll {
				// Get all files from targets metadata
				resp, err := client.get("/metadata/targets.json")
				if err != nil {
					return fmt.Errorf("failed to get targets metadata: %w", err)
				}
				defer resp.Body.Close()

				var targets TargetsMetadata
				if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
					return fmt.Errorf("failed to decode targets metadata: %w", err)
				}

				for path := range targets.Signed.Targets {
					paths = append(paths, path)
				}
			}

			if len(paths) == 0 {
				return fmt.Errorf("no paths specified (use --all to verify all targets)")
			}

			for _, path := range paths {
				result, err := client.VerifyFile(path)
				if err != nil {
					result = &VerifyResult{
						Path:  path,
						Valid: false,
					}
				}
				results = append(results, result)
			}

			// Output results
			if len(results) == 1 {
				return app.output(results[0])
			}
			return app.output(results)
		},
	}

	cmd.Flags().BoolVarP(&checkAll, "all", "a", false, "Verify all files in targets metadata")

	return cmd
}

// newRotateKeysCommand creates the rotate-keys command
func (app *App) newRotateKeysCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-keys <key-type>",
		Short: "Rotate signing keys",
		Long:  "Rotate TUF signing keys (root, targets, snapshot, timestamp)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			keyType := args[0]
			validTypes := map[string]bool{
				"root":      true,
				"targets":   true,
				"snapshot":  true,
				"timestamp": true,
			}

			if !validTypes[keyType] {
				return fmt.Errorf("invalid key type: %s (must be root, targets, snapshot, or timestamp)", keyType)
			}

			client := app.getClient()
			result, err := client.RotateKeys(keyType)
			if err != nil {
				return fmt.Errorf("key rotation failed: %w", err)
			}

			return app.output(result)
		},
	}
	return cmd
}

// newGenerateAPIKeyCommand creates the generate-api-key command
func (app *App) newGenerateAPIKeyCommand() *cobra.Command {
	var name string
	var role string
	var permissions []string

	cmd := &cobra.Command{
		Use:   "generate-api-key",
		Short: "Generate a new API key",
		Long:  "Generate a new API key for authentication with the TUF server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("name is required")
			}

			client := app.getClient()
			result, err := client.GenerateAPIKey(name, role, permissions)
			if err != nil {
				return fmt.Errorf("failed to generate API key: %w", err)
			}

			// Special handling for successful key generation
			if result.Success && app.config.OutputFormat == "text" {
				fmt.Printf("API Key generated successfully!\n")
				fmt.Printf("Name: %s\n", result.Name)
				fmt.Printf("Key: %s\n", result.APIKey)
				fmt.Printf("\nStore this key securely - it cannot be retrieved again.\n")
				return nil
			}

			return app.output(result)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "Name for the API key (required)")
	cmd.Flags().StringVarP(&role, "role", "r", "read", "Role for the API key (read, write, admin)")
	cmd.Flags().StringSliceVarP(&permissions, "permissions", "p", []string{}, "Specific permissions for the API key")
	_ = cmd.MarkFlagRequired("name")

	return cmd
}

// newLoginCommand creates the login command
func (app *App) newLoginCommand() *cobra.Command {
	var username string
	var password string
	var saveToken bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login to the TUF server",
		Long:  "Authenticate with the TUF server and obtain a JWT token",
		RunE: func(cmd *cobra.Command, args []string) error {
			if username == "" {
				fmt.Print("Username: ")
				_, _ = fmt.Scanln(&username)
			}

			if password == "" {
				fmt.Print("Password: ")
				// TODO: Use terminal package for hidden password input
				_, _ = fmt.Scanln(&password)
			}

			client := app.getClient()
			result, err := client.Login(username, password)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			if result.Success && saveToken {
				// Save token to config file
				viper.Set("jwt_token", result.Token)
				configFile := viper.ConfigFileUsed()
				if configFile == "" {
					home, _ := os.UserHomeDir()
					configFile = filepath.Join(home, ".tuf-cli.yaml")
				}
				if err := viper.WriteConfigAs(configFile); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to save token to config: %v\n", err)
				} else {
					fmt.Printf("Token saved to %s\n", configFile)
				}
			}

			return app.output(result)
		},
	}

	cmd.Flags().StringVarP(&username, "username", "u", "", "Username for authentication")
	cmd.Flags().StringVarP(&password, "password", "p", "", "Password for authentication")
	cmd.Flags().BoolVarP(&saveToken, "save", "s", false, "Save token to config file")

	return cmd
}

// newConfigCommand creates the config command
func (app *App) newConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage CLI configuration",
		Long:  "View and manage TUF CLI configuration settings",
	}

	// Subcommand to show current config
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.output(app.config)
		},
	}

	// Subcommand to set config values
	setCmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			viper.Set(key, value)
			
			configFile := viper.ConfigFileUsed()
			if configFile == "" {
				home, _ := os.UserHomeDir()
				configFile = filepath.Join(home, ".tuf-cli.yaml")
			}
			
			if err := viper.WriteConfigAs(configFile); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("Configuration updated: %s = %s\n", key, value)
			fmt.Printf("Saved to %s\n", configFile)
			return nil
		},
	}

	cmd.AddCommand(showCmd, setCmd)
	return cmd
}

// Helper functions for output formatting

func outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func outputYAML(data interface{}) error {
	encoder := yaml.NewEncoder(os.Stdout)
	return encoder.Encode(data)
}

func outputText(data interface{}) error {
	switch v := data.(type) {
	case *ServerStatus:
		fmt.Printf("Server Status: %s\n", v.Status)
		fmt.Printf("Version: %s\n", v.Version)
		fmt.Printf("Repository: %s\n", v.Repository)
		fmt.Printf("Uptime: %s\n", v.Uptime)
		if len(v.Cache) > 0 {
			fmt.Println("\nCache Info:")
			for key, value := range v.Cache {
				fmt.Printf("  %s: %v\n", key, value)
			}
		}
		if len(v.Metrics) > 0 {
			fmt.Println("\nMetrics:")
			for key, value := range v.Metrics {
				fmt.Printf("  %s: %v\n", key, value)
			}
		}

	case *ConnectivityResult:
		fmt.Printf("Server URL: %s\n", v.ServerURL)
		fmt.Printf("Connected: %v\n", v.Connected)
		fmt.Printf("Health Status: %d\n", v.HealthStatus)
		fmt.Printf("Ready Status: %d\n", v.ReadyStatus)
		fmt.Printf("Admin Access: %v\n", v.AdminAccess)
		fmt.Printf("Timestamp: %s\n", v.Timestamp.Format("2006-01-02 15:04:05"))
		if v.Error != "" {
			fmt.Printf("Error: %s\n", v.Error)
		}

	case *UploadResult:
		if v.Success {
			fmt.Printf("✓ Uploaded %s -> %s (%d bytes)\n", v.LocalPath, v.RemotePath, v.Size)
		} else {
			fmt.Printf("✗ Failed to upload %s: %s\n", v.LocalPath, v.Error)
		}

	case []*UploadResult:
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Status\tLocal Path\tRemote Path\tSize\tError")
		for _, r := range v {
			status := "✗"
			if r.Success {
				status = "✓"
			}
			errMsg := r.Error
			if errMsg == "" {
				errMsg = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", status, r.LocalPath, r.RemotePath, r.Size, errMsg)
		}
		w.Flush()

	case *DeleteResult:
		if v.Success {
			fmt.Printf("✓ Deleted %s\n", v.Path)
		} else {
			fmt.Printf("✗ Failed to delete %s: %s\n", v.Path, v.Error)
		}

	case []*DeleteResult:
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Status\tPath\tError")
		for _, r := range v {
			status := "✗"
			if r.Success {
				status = "✓"
			}
			errMsg := r.Error
			if errMsg == "" {
				errMsg = "-"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", status, r.Path, errMsg)
		}
		w.Flush()

	case *VerifyResult:
		fmt.Printf("Path: %s\n", v.Path)
		fmt.Printf("Exists: %v\n", v.Exists)
		if v.Exists {
			fmt.Printf("Size: %d bytes\n", v.Size)
			fmt.Printf("Checksum: %s\n", v.Checksum)
			fmt.Printf("In Metadata: %v\n", v.InMetadata)
			if v.InMetadata {
				fmt.Printf("Metadata Checksum: %s\n", v.MetadataChecksum)
				fmt.Printf("Valid: %v\n", v.Valid)
			}
		}

	case []*VerifyResult:
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Status\tPath\tSize\tIn Metadata\tValid")
		for _, r := range v {
			status := "✗"
			if r.Valid {
				status = "✓"
			} else if !r.Exists {
				status = "?"
			}
			inMeta := "-"
			if r.InMetadata {
				inMeta = "Yes"
			}
			valid := "-"
			if r.InMetadata {
				if r.Valid {
					valid = "Yes"
				} else {
					valid = "No"
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n", status, r.Path, r.Size, inMeta, valid)
		}
		w.Flush()

	case *LoginResult:
		if v.Success {
			fmt.Printf("✓ Login successful\n")
			fmt.Printf("Token: %s...\n", v.Token[:20])
			fmt.Printf("Expires: %s\n", v.ExpiresAt)
		} else {
			fmt.Printf("✗ Login failed: %s\n", v.Error)
		}

	case *APIKeyResult:
		// Handled specially in the command
		return outputJSON(v)

	case *KeyRotationResult:
		if v.Success {
			fmt.Printf("✓ Key rotation successful\n")
			fmt.Printf("Key Type: %s\n", v.KeyType)
			if v.OldKeyID != "" {
				fmt.Printf("Old Key ID: %s\n", v.OldKeyID)
			}
			if v.NewKeyID != "" {
				fmt.Printf("New Key ID: %s\n", v.NewKeyID)
			}
		} else {
			fmt.Printf("✗ Key rotation failed: %s\n", v.Error)
		}

	case *Config:
		fmt.Printf("Server URL: %s\n", v.ServerURL)
		fmt.Printf("API Key: %s\n", maskString(v.APIKey))
		fmt.Printf("JWT Token: %s\n", maskString(v.JWTToken))
		fmt.Printf("Output Format: %s\n", v.OutputFormat)
		fmt.Printf("Verbose: %v\n", v.Verbose)
		if v.ConfigFile != "" {
			fmt.Printf("Config File: %s\n", v.ConfigFile)
		}

	default:
		return outputJSON(data)
	}
	return nil
}

// maskString masks sensitive strings for display
func maskString(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 8 {
		return strings.Repeat("*", len(s))
	}
	return s[:4] + strings.Repeat("*", len(s)-8) + s[len(s)-4:]
}

// SHA256Hasher wraps sha256 hash computation
type SHA256Hasher struct {
	hash hash.Hash
}

func newSHA256Hasher() *SHA256Hasher {
	return &SHA256Hasher{
		hash: sha256.New(),
	}
}

func (h *SHA256Hasher) Write(p []byte) (n int, err error) {
	return h.hash.Write(p)
}

func (h *SHA256Hasher) Sum() string {
	return hex.EncodeToString(h.hash.Sum(nil))
}