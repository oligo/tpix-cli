package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/typstify/tpix-cli/api"
	"github.com/typstify/tpix-cli/bundler"
	"github.com/typstify/tpix-cli/config"
	"github.com/typstify/tpix-cli/deps"
	"github.com/typstify/tpix-cli/version"
)

// parsePkgSpec parses a package spec in the format @namespace/name:version
// Returns namespace, name, and version (version may be empty)
func parsePkgSpec(pkgSpec string) (namespace, name, version string) {
	// Remove leading @ and split on /
	s := strings.TrimPrefix(pkgSpec, "@")
	parts := strings.SplitN(s, "/", 2)
	if len(parts) < 2 {
		return
	}
	namespace = parts[0]

	// Split name and version on :
	nameVer := strings.SplitN(parts[1], ":", 2)
	name = nameVer[0]
	if len(nameVer) > 1 {
		version = nameVer[1]
	}
	return
}

func loginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login the tpix server",
		Long:  "Login the tpix server. User is required to login for all other operations",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			tokenResp, err := api.DeviceLogin()
			if err != nil {
				fmt.Printf("Login failed: %v\n", err)
				return err
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			cfg.AccessToken = tokenResp.AccessToken
			cfg.RefreshToken = tokenResp.RefreshToken
			config.Save(cfg)
			fmt.Printf("\n\nSuccess! Access token saved\n")

			return nil
		},
	}

	return cmd
}

// searchPkgCmd searches Typst packages from TPIX server.
func searchPkgCmd() *cobra.Command {
	var namespace string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for Typst packages",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]

			result, err := api.SearchPackages(query, namespace, limit)
			if err != nil {
				fmt.Printf("failed to search packages: %v", err)
				return nil
			}

			fmt.Printf("Found %d results for '%s':\n\n", result.Count, query)
			for _, r := range result.Results {
				fmt.Printf("@%s/%s - %s\n", r.Namespace, r.Name, r.Description)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Filter by namespace")
	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "Limit number of results")

	return cmd
}

// isPackageCached checks if a package version is already in the local cache.
func isPackageCached(cacheDir, namespace, name, version string) bool {
	pkgDir := filepath.Join(cacheDir, namespace, name, version)
	info, err := os.Stat(pkgDir)
	return err == nil && info.IsDir()
}

// fetchWithDeps downloads a package and its transitive dependencies.
// visited tracks already-processed packages to prevent infinite loops.
func fetchWithDeps(namespace, name, version, cacheDir string, visited map[string]bool, noDeps bool) error {
	key := fmt.Sprintf("@%s/%s:%s", namespace, name, version)
	if visited[key] {
		return nil
	}
	visited[key] = true

	if isPackageCached(cacheDir, namespace, name, version) {
		fmt.Printf("  Already cached: %s\n", key)
		// Do not return early, check if dependencies are satisfied.
	} else {
		fmt.Printf("  Downloading %s...\n", key)
		if err := api.DownloadPackage(namespace, name, version); err != nil {
			return fmt.Errorf("failed to download %s: %w", key, err)
		}
	}

	if noDeps {
		return nil
	}

	// Fetch and resolve transitive dependencies
	depInfos, err := api.FetchDependencies(namespace, name, version)
	if err != nil {
		// Non-fatal: the server may not have dependency data for older packages
		return nil
	}

	for _, dep := range depInfos {
		if err := fetchWithDeps(dep.Namespace, dep.Name, dep.Version, cacheDir, visited, false); err != nil {
			return err
		}
	}

	return nil
}

// getPkgCmd download Typst packages from TPIX server.
func getPkgCmd() *cobra.Command {
	var noDeps bool

	cmd := &cobra.Command{
		Use:   "get <namespace/name:version>",
		Short: "Download a package from TPIX server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgSpec := args[0]

			// Parse namespace/name:version
			namespace, name, version := parsePkgSpec(pkgSpec)

			if version == "" {
				// Get latest version first
				pkg, err := api.FetchPackage(namespace, name)
				if err != nil {
					return err
				}
				if len(pkg.Versions) == 0 {
					return fmt.Errorf("no versions available for package")
				}
				version = pkg.Versions[len(pkg.Versions)-1].Version
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			fmt.Printf("Resolving @%s/%s:%s...\n", namespace, name, version)
			visited := make(map[string]bool)
			if err := fetchWithDeps(namespace, name, version, cacheDir, visited, noDeps); err != nil {
				return err
			}

			fmt.Printf("Done. %d package(s) resolved.\n", len(visited))
			return nil
		},
	}

	cmd.Flags().BoolVar(&noDeps, "no-deps", false, "Skip fetching transitive dependencies")

	return cmd
}

// pullCmd scans the current project for .typ imports and fetches all dependencies.
func pullCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fetch all package dependencies for the current project",
		Long: `Scan the current directory recursively for .typ files, discover all
#import "@namespace/name:version" references, and download each package
along with its transitive dependencies.

Use --dry-run to see what would be fetched without downloading anything.`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			// Scan current directory for .typ imports
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get working directory: %w", err)
			}

			fmt.Printf("Scanning %s for package imports...\n", cwd)
			discovered, err := deps.ExtractFromDirectory(cwd)
			if err != nil {
				return fmt.Errorf("failed to scan for imports: %w", err)
			}

			if len(discovered) == 0 {
				fmt.Println("No package imports found.")
				return nil
			}

			fmt.Printf("Found %d direct dependency(ies).\n", len(discovered))

			if dryRun {
				for _, dep := range discovered {
					cached := isPackageCached(cacheDir, dep.Namespace, dep.Name, dep.Version)
					status := "missing"
					if cached {
						status = "cached"
					}
					fmt.Printf("  %s [%s]\n", dep.Key(), status)
				}
				return nil
			}

			visited := make(map[string]bool)
			for _, dep := range discovered {
				if err := fetchWithDeps(dep.Namespace, dep.Name, dep.Version, cacheDir, visited, false); err != nil {
					return err
				}
			}

			fmt.Printf("Done. %d package(s) resolved.\n", len(visited))
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be fetched without downloading")

	return cmd
}

// listCachedCmd lists locally cached/downloaded packages.
func listCachedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List locally cached packages",
		Long:  "List all packages downloaded and cached in the local package cache",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			entries, err := os.ReadDir(cacheDir)
			if err != nil {
				return fmt.Errorf("failed to read cache directory: %w", err)
			}

			var count int
			fmt.Printf("Cached packages in %s:\n\n", cacheDir)

			for _, namespace := range entries {
				if !namespace.IsDir() {
					continue
				}
				namespacePath := filepath.Join(cacheDir, namespace.Name())
				pkgs, err := os.ReadDir(namespacePath)
				if err != nil {
					continue
				}
				for _, pkg := range pkgs {
					if !pkg.IsDir() {
						continue
					}
					pkgPath := filepath.Join(namespacePath, pkg.Name())
					versions, err := os.ReadDir(pkgPath)
					if err != nil {
						continue
					}
					for _, version := range versions {
						if !version.IsDir() {
							continue
						}
						count++
						fmt.Printf("@%s/%s:%s\n", namespace.Name(), pkg.Name(), version.Name())
					}
				}
			}

			fmt.Printf("\nTotal: %d packages\n", count)

			return nil
		},
	}

	return cmd
}

// removeCachedCmd removes a cached package.
func removeCachedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <namespace/name:version>",
		Short: "Remove a cached package",
		Long:  "Remove a locally cached package from the cache directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgSpec := args[0]
			namespace, name, version := parsePkgSpec(pkgSpec)

			if namespace == "" || name == "" || version == "" {
				return fmt.Errorf("invalid package spec: use format @namespace/name:version")
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("typst cache directory not configured")
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("typst cache directory not configured")
			}

			pkgDir := filepath.Join(cacheDir, namespace, name, version)

			// Check if the package exists
			info, err := os.Stat(pkgDir)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("package @%s/%s:%s not found in cache", namespace, name, version)
				}
				return fmt.Errorf("failed to check package: %v", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("package @%s/%s:%s is not a directory", namespace, name, version)
			}

			if err := os.RemoveAll(pkgDir); err != nil {
				return fmt.Errorf("failed to remove package: %v", err)
			}

			fmt.Printf("Removed @%s/%s:%s from cache\n", namespace, name, version)
			return nil
		},
	}

	return cmd
}

// queryPkgCmd query package detail from TPIX server.
func queryPkgCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <namespace/name>",
		Short: "Show detailed information about a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgSpec := args[0]

			// Parse namespace/name
			namespace, name, _ := parsePkgSpec(pkgSpec)

			pkg, err := api.FetchPackage(namespace, name)
			if err != nil {
				return err
			}

			fmt.Printf("Package: @%s/%s\n\n", namespace, name)
			fmt.Printf("Description: %s\n", pkg.Description)
			fmt.Printf("Website: %s\n", pkg.HomepageURL)
			fmt.Printf("Repository: %s\n", pkg.RepositoryURL)
			fmt.Printf("License: %s\n", pkg.License)
			fmt.Printf("\nVersions:\n")
			for _, v := range pkg.Versions {
				fmt.Printf("  %s (Typst: %s)\n", v.Version, v.TypstVersion)
			}

			return nil
		},
	}

	return cmd
}

// bundleCmd creates a Typst package from a directory.
func bundleCmd() *cobra.Command {
	var output string
	var exclude []string

	cmd := &cobra.Command{
		Use:   "bundle <directory>",
		Short: "Create a Typst package from a directory",
		Long: `Create a .tar.gz Typst package from a directory containing a typst.toml manifest.
The directory must contain a valid typst.toml file with required fields:
- package.name
- package.version
- package.entrypoint

Files and directories can be excluded using the --exclude flag or the exclude field in typst.toml.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			srcDir := args[0]

			// Check if directory exists
			info, err := os.Stat(srcDir)
			if err != nil {
				return fmt.Errorf("failed to access directory: %w", err)
			}
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", srcDir)
			}

			// Check for typst.toml
			manifestPath := filepath.Join(srcDir, "typst.toml")
			if _, err := os.Stat(manifestPath); err != nil {
				return fmt.Errorf("typst.toml not found in %s - a valid manifest is required", srcDir)
			}

			// Determine output path
			if output == "" {
				// Use directory name with .tar.gz extension
				output = filepath.Base(srcDir) + ".tar.gz"
			}

			// Create package
			creator := bundler.NewPackageCreator(exclude)
			if err := creator.CreatePackage(srcDir, output); err != nil {
				return fmt.Errorf("failed to create package: %w", err)
			}

			fmt.Printf("Package created: %s\n", output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: <directory>.tar.gz)")
	cmd.Flags().StringSliceVarP(&exclude, "exclude", "e", []string{}, "Additional files/directories to exclude")

	return cmd
}

// pushCmd uploads a package to the TPIX server.
func pushCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push <package.tar.gz> <namespace>",
		Short: "Upload a package to the TPIX server",
		Long: `Upload a .tar.gz Typst package to the TPIX server.
The package must be a valid Typst package archive created with the bundle command.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			packagePath := args[0]
			namespace := args[1]

			// Check if file exists
			info, err := os.Stat(packagePath)
			if err != nil {
				return fmt.Errorf("failed to access package: %w", err)
			}
			if info.IsDir() {
				return fmt.Errorf("%s is a directory, not a package file", packagePath)
			}

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Check if user is logged in
			if cfg.AccessToken == "" {
				return fmt.Errorf("not logged in. Please run 'tpix login' first")
			}

			fmt.Printf("Uploading %s to namespace %s...\n", packagePath, namespace)

			resp, err := api.UploadPackage(packagePath, namespace)
			if err != nil {
				return fmt.Errorf("upload failed: %w", err)
			}

			if resp.SHA256 != "" {
				fmt.Printf("Successfully uploaded package: @%s/%s:%s\n", namespace, resp.Package, resp.Version)
			} else {
				fmt.Printf("Upload failed, report: \n")
				for _, r := range resp.ValidateReport {
					fmt.Printf("\t%s\n", r)
				}
			}

			return nil
		},
	}

	return cmd
}

// versionCmd shows the current version and checks for updates.
func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  "Show the current version of tpix-cli and check for available updates",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("tpix-cli version %s\n", version.FormatedVersion())

			// Check for updates
			updater := &version.Updater{}
			hasUpdate, err := updater.Check()
			if err != nil {
				// Don't fail if update check fails, just warn
				fmt.Printf("\nWarning: could not check for updates: %v\n", err)
				return nil
			}

			if hasUpdate {
				latest, err := updater.Latest()
				if err != nil {
					fmt.Printf("\nWarning: could not get latest version info: %v\n", err)
					return nil
				}
				fmt.Printf("\nA new version is available: %s\n", latest.Version)
				fmt.Printf("Run 'tpix update' to upgrade\n")
			} else {
				fmt.Printf("\nYou are running the latest version.\n")
			}

			return nil
		},
	}

	return cmd
}

// updateCmd upgrades tpix-cli to the latest version.
func updateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update tpix-cli to the latest version",
		Long:  "Download and install the latest version of tpix-cli from GitHub releases",
		Args:  cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Checking for updates...")

			updater := &version.Updater{}
			hasUpdate, err := updater.Check()
			if err != nil {
				return fmt.Errorf("failed to check for updates: %w", err)
			}

			if !hasUpdate {
				fmt.Println("You are already running the latest version.")
				return nil
			}

			latest, err := updater.Latest()
			if err != nil {
				return fmt.Errorf("failed to get latest version info: %w", err)
			}

			fmt.Printf("Downloading version %s...\n", latest.Version)

			progress, err := updater.Update()
			if err != nil {
				return fmt.Errorf("failed to update: %w", err)
			}

			// Wait for download to complete
			for ratio := range progress.Progress() {
				// Simple progress indicator
				fmt.Printf("\rDownloading... %.1f%%", ratio*100)
			}
			fmt.Println("\rDownloading... 100%")

			if progress.Err != nil {
				return fmt.Errorf("download failed: %w", progress.Err)
			}

			fmt.Printf("\nSuccessfully updated to version %s\n", latest.Version)

			return nil
		},
	}

	return cmd
}

// cachePathCmd prints the cache directory path.
func cachePathCmd() *cobra.Command {
	var setPath string

	cmd := &cobra.Command{
		Use:   "cache-path",
		Short: "Print or set the cache directory path",
		Long: `Print or set the path where Typst packages are cached.

The cache path can be set via:
  1. The --set flag: tpix cache-path --set /custom/path
  2. The TYPST_PACKAGE_CACHE_PATH environment variable

If neither is set, the default path is used:
  - Linux/macOS: ~/.cache/typst/packages
  - Windows: %LOCALAPPDATA%\typst\packages`,
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			flagSet := cmd.Flags().Changed("set")

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if flagSet {
				// Flag was explicitly set
				if setPath == "" {
					// Empty string - clear and let Save() use detected default
					cfg.TypstCachePkgPath = ""
					if err := config.Save(cfg); err != nil {
						return fmt.Errorf("failed to save config: %w", err)
					}
					cfg, _ = config.Load()

					fmt.Printf("Cache path reset to: %s\n", cfg.TypstCachePkgPath)
					return nil
				}

				// Validate path - check if it exists and is a directory
				info, err := os.Stat(setPath)
				if err != nil {
					if os.IsNotExist(err) {
						return fmt.Errorf("path does not exist: %s", setPath)
					}
					return fmt.Errorf("invalid path: %w", err)
				}
				if !info.IsDir() {
					return fmt.Errorf("path is not a directory: %s", setPath)
				}

				cfg.TypstCachePkgPath = setPath
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
				cfg, _ = config.Load()

				fmt.Printf("Cache path set to: %s\n", cfg.TypstCachePkgPath)
				return nil
			}

			cacheDir := cfg.TypstCachePkgPath
			if cacheDir == "" {
				return fmt.Errorf("cache directory not configured")
			}
			fmt.Println(cacheDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&setPath, "set", "", "Set a custom cache path")

	return cmd
}
