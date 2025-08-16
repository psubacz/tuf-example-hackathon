package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// App represents the CLI application
type App struct {
	rootCmd *cobra.Command
	config  *Config
}

// Config holds CLI configuration
type Config struct {
	ServerURL   string `mapstructure:"server_url"`
	APIKey      string `mapstructure:"api_key"`
	JWTToken    string `mapstructure:"jwt_token"`
	ConfigFile  string `mapstructure:"config_file"`
	OutputFormat string `mapstructure:"output_format"`
	Verbose     bool   `mapstructure:"verbose"`
}

// NewApp creates a new CLI application
func NewApp() *App {
	app := &App{
		config: &Config{},
	}

	app.rootCmd = &cobra.Command{
		Use:   "tuf-cli",
		Short: "TUF Server CLI Admin Tool",
		Long: `TUF CLI is a command-line tool for managing TUF repositories.
It provides administrative functions for uploading, deleting, and verifying files,
managing API keys, rotating keys, and checking server status.`,
		PersistentPreRunE: app.initConfig,
	}

	// Global flags
	app.rootCmd.PersistentFlags().StringVar(&app.config.ServerURL, "server", "http://localhost:8080", "TUF server URL")
	app.rootCmd.PersistentFlags().StringVar(&app.config.APIKey, "api-key", "", "API key for authentication")
	app.rootCmd.PersistentFlags().StringVar(&app.config.JWTToken, "jwt", "", "JWT token for authentication")
	app.rootCmd.PersistentFlags().StringVar(&app.config.ConfigFile, "config", "", "Config file (default: $HOME/.tuf-cli.yaml)")
	app.rootCmd.PersistentFlags().StringVarP(&app.config.OutputFormat, "output", "o", "text", "Output format (text, json, yaml)")
	app.rootCmd.PersistentFlags().BoolVarP(&app.config.Verbose, "verbose", "v", false, "Verbose output")

	// Bind flags to viper
	_ = viper.BindPFlag("server_url", app.rootCmd.PersistentFlags().Lookup("server"))
	_ = viper.BindPFlag("api_key", app.rootCmd.PersistentFlags().Lookup("api-key"))
	_ = viper.BindPFlag("jwt_token", app.rootCmd.PersistentFlags().Lookup("jwt"))
	_ = viper.BindPFlag("output_format", app.rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("verbose", app.rootCmd.PersistentFlags().Lookup("verbose"))

	// Add commands
	app.addCommands()

	return app
}

// Run executes the CLI application
func (app *App) Run(args []string) error {
	return app.rootCmd.Execute()
}

// initConfig initializes the configuration
func (app *App) initConfig(cmd *cobra.Command, args []string) error {
	// Set config file
	if app.config.ConfigFile != "" {
		viper.SetConfigFile(app.config.ConfigFile)
	} else {
		// Look for config in home directory
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
			viper.SetConfigName(".tuf-cli")
			viper.SetConfigType("yaml")
		}
	}

	// Environment variables
	viper.SetEnvPrefix("TUF_CLI")
	viper.AutomaticEnv()

	// Read config file if it exists
	if err := viper.ReadInConfig(); err == nil {
		if app.config.Verbose {
			fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
		}
	}

	// Unmarshal config
	if err := viper.Unmarshal(app.config); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required config
	if app.config.ServerURL == "" {
		return fmt.Errorf("server URL is required")
	}

	return nil
}

// addCommands adds all subcommands to the root command
func (app *App) addCommands() {
	app.rootCmd.AddCommand(
		app.newStatusCommand(),
		app.newConnectivityCommand(),
		app.newUploadCommand(),
		app.newDeleteCommand(),
		app.newVerifyCommand(),
		app.newRotateKeysCommand(),
		app.newGenerateAPIKeyCommand(),
		app.newLoginCommand(),
		app.newConfigCommand(),
	)
}

// getClient returns an HTTP client configured for the TUF server
func (app *App) getClient() *Client {
	return NewClient(app.config)
}

// output formats and prints the result based on the output format
func (app *App) output(data interface{}) error {
	switch app.config.OutputFormat {
	case "json":
		return outputJSON(data)
	case "yaml":
		return outputYAML(data)
	default:
		return outputText(data)
	}
}

