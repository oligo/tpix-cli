package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oligo/tpix-cli/api"
	"github.com/oligo/tpix-cli/bundler"
	"github.com/oligo/tpix-cli/config"
	"github.com/spf13/cobra"
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
			token, err := api.DeviceLogin()
			if err != nil {
				fmt.Printf("Login failed: %v\n", err)
				return err
			}

			config.AppConfig.AccessToken = token
			config.Save()
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

// getPkgCmd download Typst packages from TPIX server.
func getPkgCmd() *cobra.Command {
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

			fmt.Printf("Downloading @%s/%s version %s...\n", namespace, name, version)

			if err := api.DownloadPackage(namespace, name, version); err != nil {
				return err
			}

			cacheDir := config.AppConfig.TypstCachePkgPath

			if cacheDir != "" {
				fmt.Printf("Package extracted to: %s\n", filepath.Join(cacheDir, namespace, name, version))
			}
			return nil
		},
	}

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
			cacheDir := config.AppConfig.TypstCachePkgPath
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
		Run: func(cmd *cobra.Command, args []string) {
			pkgSpec := args[0]
			namespace, name, version := parsePkgSpec(pkgSpec)

			if namespace == "" || name == "" || version == "" {
				fmt.Println("invalid package spec: use format @namespace/name:version")
				return
			}

			cacheDir := config.AppConfig.TypstCachePkgPath
			if cacheDir == "" {
				fmt.Println("typst cache directory not configured")
				return
			}

			pkgDir := filepath.Join(cacheDir, namespace, name, version)

			// Check if the package exists
			info, err := os.Stat(pkgDir)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Printf("package @%s/%s:%s not found in cache", namespace, name, version)
					return
				}
				fmt.Printf("failed to check package: %v", err)
				return
			}
			if !info.IsDir() {
				fmt.Printf("package @%s/%s:%s is not a directory", namespace, name, version)
				return
			}

			if err := os.RemoveAll(pkgDir); err != nil {
				fmt.Printf("failed to remove package: %v", err)
				return
			}

			fmt.Printf("Removed @%s/%s:%s from cache\n", namespace, name, version)
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

			// Check if user is logged in
			if config.AppConfig.AccessToken == "" {
				return fmt.Errorf("not logged in. Please run 'tpix-cli login' first")
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
