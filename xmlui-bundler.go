package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	repoName     = "xmlui-invoice"
	branchName   = "main"
	appZipURL    = "https://codeload.github.com/jonudell/" + repoName + "/zip/refs/heads/" + branchName
	xmluiRepoZip = "https://codeload.github.com/xmlui-com/xmlui/zip/refs/heads/main"
)

func getPlatformSpecificMCPURL() string {
	baseURL := "https://github.com/jonudell/xmlui-mcp/releases/download/v1.0.0/"
	arch := runtime.GOARCH
	switch runtime.GOOS {
	case "darwin":
		if arch == "arm64" {
			return baseURL + "xmlui-mcp-mac-arm.tar.gz"
		}
		return baseURL + "xmlui-mcp-mac-amd.tar.gz"
	case "linux":
		return baseURL + "xmlui-mcp-linux-amd64.zip"
	case "windows":
		return baseURL + "xmlui-mcp-windows-amd64.zip"
	default:
		return baseURL + "xmlui-mcp-mac-arm.tar.gz"
	}
}

func getPlatformSpecificServerURL() string {
	baseURL := "https://github.com/JonUdell/xmlui-test-server/releases/download/v1.0.0/"
	arch := runtime.GOARCH
	switch runtime.GOOS {
	case "darwin":
		if arch == "arm64" {
			return baseURL + "xmlui-test-server-mac-arm.tar.gz"
		}
		return baseURL + "xmlui-test-server-mac-amd.tar.gz"
	case "linux":
		return baseURL + "xmlui-test-server-linux-amd64.tar.gz"
	case "windows":
		return baseURL + "xmlui-test-server-windows-amd64.zip"
	default:
		return baseURL + "xmlui-test-server-mac-arm.tar.gz"
	}
}

func downloadWithProgress(url, filename string) ([]byte, error) {
	fmt.Printf("Downloading %s...\n", filename)
	fmt.Printf("  From: %s\n", url)

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if strings.Contains(url, "codeload.github.com/xmlui-com/xmlui") {
		token := os.Getenv("GITHUB_TOKEN")
		if token != "" {
			fmt.Println("  Using authentication token for private repository")
			req.SetBasicAuth(token, "x-oauth-basic")
		} else {
			fmt.Println("  Warning: No authentication token found for private repository")
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if strings.Contains(url, "codeload.github.com/xmlui-com/xmlui") && resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("authentication failed for private repository: %s (status: %s) - check PAT_TOKEN", url, resp.Status)
		}
		return nil, fmt.Errorf("request failed: %s for URL: %s", resp.Status, url)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	fmt.Printf("  Downloaded: %d bytes\n", len(data))
	return data, nil
}

func unzipTo(data []byte, dest string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(fpath)
		if err != nil {
			return err
		}
		io.Copy(out, in)
		in.Close()
		out.Close()
	}
	return nil
}

func untarGzTo(data []byte, dest string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	tarReader := tar.NewReader(gzReader)
	for {
		hdr, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fpath := filepath.Join(dest, hdr.Name)
		if hdr.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
		out, err := os.Create(fpath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tarReader); err != nil {
			return err
		}
		out.Close()

		// Set executable bit for script files and binaries
		if strings.HasSuffix(fpath, ".sh") || filepath.Base(fpath) == "xmlui-mcp" ||
		   filepath.Base(fpath) == "xmlui-mcp-client" || filepath.Base(fpath) == "xmlui-test-server" {
			os.Chmod(fpath, 0755)
			// Note: No need to remove quarantine on macOS for tar.gz files
			// as the attribute won't be set on extraction
		}
	}
	return nil
}

func moveIntoPlace(srcParent, repoName, installDir string) (string, error) {
	repoPrefix := repoName + "-"
	entries, err := os.ReadDir(srcParent)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), repoPrefix) {
			tmp := filepath.Join(srcParent, e.Name())
			final := filepath.Join(installDir, repoName)
			if err := os.Rename(tmp, final); err != nil {
				return "", err
			}
			return final, nil
		}
	}
	return "", fmt.Errorf("repo dir not found")
}

func main() {
	installDir, _ := os.Getwd()
	os.MkdirAll(installDir, 0755)

	fmt.Println("Step 1/5: Downloading XMLUI invoice app...")
	appZip, err := downloadWithProgress(appZipURL, "XMLUI invoice app")
	if err != nil {
		fmt.Println("Failed to download app:", err)
		os.Exit(1)
	}
	if err := unzipTo(appZip, installDir); err != nil {
		fmt.Println("Failed to extract app:", err)
		os.Exit(1)
	}

	appDir, err := moveIntoPlace(installDir, repoName, installDir)
	if err != nil {
		fmt.Println("Failed to organize app directory:", err)
		os.Exit(1)
	}

	fmt.Println("Step 2/5: Downloading XMLUI components...")
	xmluiZip, err := downloadWithProgress(xmluiRepoZip, "XMLUI repo")
	if err != nil {
		fmt.Println("Failed to download XMLUI source:", err)
		os.Exit(1)
	}
	// Extract XMLUI components and place them in the mcp/docs and mcp/src directories
	tmpDir := filepath.Join(installDir, "xmlui-source")
	os.MkdirAll(tmpDir, 0755)
	if err := unzipTo(xmluiZip, tmpDir); err != nil {
		fmt.Println("Failed to extract XMLUI source:", err)
		os.Exit(1)
	}

	// Find the root of the extracted XMLUI source
	var sourceRoot string
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "xmlui-") {
			sourceRoot = filepath.Join(tmpDir, e.Name())
			break
		}
	}

	// Setup mcp dir with docs and src
	mcpDir := filepath.Join(installDir, "mcp")
	os.MkdirAll(mcpDir, 0755)

	// First ensure docs and src directories are created under mcp
	docsDir := filepath.Join(mcpDir, "docs")
	srcDir := filepath.Join(mcpDir, "src")
	os.MkdirAll(docsDir, 0755)
	os.MkdirAll(srcDir, 0755)

	// Copy components
	if sourceRoot != "" {
		// Set up components directories
		os.MkdirAll(filepath.Join(docsDir, "pages", "components"), 0755)
		os.MkdirAll(filepath.Join(srcDir, "components"), 0755)

		// Copy component docs
		copyFiles(filepath.Join(sourceRoot, "docs", "pages", "components"), filepath.Join(docsDir, "pages", "components"))

		// Copy component source
		copyFiles(filepath.Join(sourceRoot, "xmlui", "src", "components"), filepath.Join(srcDir, "components"))

		fmt.Println("✓ Extracted components")
	}

	// Clean up the source directory
	_ = os.RemoveAll(tmpDir)

	fmt.Println("Step 3/5: Downloading MCP tools...")
	mcpUrl := getPlatformSpecificMCPURL()
	mcpArchive, err := downloadWithProgress(mcpUrl, "MCP tools")
	if err != nil {
		fmt.Println("Failed to download MCP tools:", err)
		os.Exit(1)
	}

	tmpMCP := filepath.Join(installDir, "mcpTmp")
	os.MkdirAll(tmpMCP, 0755)

	// Extract based on file type
	if strings.HasSuffix(mcpUrl, ".zip") {
		err = unzipTo(mcpArchive, tmpMCP)
	} else {
		err = untarGzTo(mcpArchive, tmpMCP)
	}

	if err != nil {
		fmt.Println("Failed to extract MCP tools:", err)
		os.Exit(1)
	}

	var expectedFiles []string
	if runtime.GOOS == "windows" {
		expectedFiles = []string{"xmlui-mcp.exe", "xmlui-mcp-client.exe", "run-mcp-client.bat"}
	} else {
		expectedFiles = []string{"xmlui-mcp", "xmlui-mcp-client", "prepare-binaries.sh", "run-mcp-client.sh"}
	}

	for _, name := range expectedFiles {
		src := filepath.Join(tmpMCP, name)
		dst := filepath.Join(mcpDir, name)
		if err := os.Rename(src, dst); err != nil {
			fmt.Printf("  Skipping %s (not found?): %v\n", name, err)
			continue
		}
		fmt.Printf("  Moved %s to %s\n", name, dst)

		// Set executable permission for non-Windows executables
		if runtime.GOOS != "windows" && (strings.HasSuffix(name, ".sh") || !strings.Contains(name, ".")) {
			os.Chmod(dst, 0755)
		}
	}

	// Clean up the temporary MCP directory
	_ = os.RemoveAll(tmpMCP)

	// Move docs and src under mcp if they exist at the root level
	if _, err := os.Stat(filepath.Join(installDir, "docs")); err == nil {
		if err := os.Rename(filepath.Join(installDir, "docs"), docsDir); err != nil {
			fmt.Printf("Warning: Could not move docs directory: %v\n", err)
		}
	}

	if _, err := os.Stat(filepath.Join(installDir, "src")); err == nil {
		if err := os.Rename(filepath.Join(installDir, "src"), srcDir); err != nil {
			fmt.Printf("Warning: Could not move src directory: %v\n", err)
		}
	}

	fmt.Println("Step 4/5: Downloading XMLUI test server...")
	serverURL := getPlatformSpecificServerURL()
	serverArchive, err := downloadWithProgress(serverURL, "test server")
	if err != nil {
		fmt.Println("Failed to download server:", err)
		os.Exit(1)
	}

	if strings.HasSuffix(serverURL, ".zip") {
		err = unzipTo(serverArchive, appDir)
	} else {
		err = untarGzTo(serverArchive, appDir)
	}

	if err != nil {
		fmt.Println("Failed to extract server:", err)
		os.Exit(1)
	}

	// Set executable permission for start.sh
	startScriptPath := filepath.Join(appDir, "start.sh")
	if runtime.GOOS != "windows" {
		os.Chmod(startScriptPath, 0755)
	}

	// The final bundle should contain only these files/directories:
	// - xmlui-invoice/  (the invoice app)
	// - mcp/  (with docs/ and src/ inside it)
	// - XMLUI_GETTING_STARTED_README.md

	// Write a cleanup script that will remove files not in the include list
	if runtime.GOOS == "windows" {
		cleanupScript := "@echo off\r\n"
		cleanupScript += "echo Cleaning up temporary files...\r\n"
		cleanupScript += fmt.Sprintf("if exist \"%s\" del \"%s\"\r\n", filepath.Base(os.Args[0]), filepath.Base(os.Args[0]))
		cleanupScript += "if exist *.zip del *.zip\r\n"
		cleanupScript += "del cleanup.bat\r\n"
		os.WriteFile(filepath.Join(installDir, "cleanup.bat"), []byte(cleanupScript), 0755)
		fmt.Println("Note: Run cleanup.bat to remove the bundler executable and temporary files")
	} else {
		cleanupScript := "#!/bin/sh\n"
		cleanupScript += "echo Cleaning up temporary files...\n"
		cleanupScript += fmt.Sprintf("rm -f \"%s\"\n", filepath.Base(os.Args[0]))
		cleanupScript += "rm -f *.zip\n"
		cleanupScript += "rm -f *.tar.gz\n"
		cleanupScript += "rm -f cleanup.sh\n"
		os.WriteFile(filepath.Join(installDir, "cleanup.sh"), []byte(cleanupScript), 0755)
		os.Chmod(filepath.Join(installDir, "cleanup.sh"), 0755)
		fmt.Println("Note: Run ./cleanup.sh to remove the bundler executable and temporary files")
	}

	fmt.Println("✓ Organized layout complete")
	fmt.Printf("\nInstall location: %s\n", installDir)
}

// copyFiles recursively copies files from src to dst directory
func copyFiles(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			os.MkdirAll(dstPath, 0755)
			if err := copyFiles(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy the file
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}

			err = os.WriteFile(dstPath, data, 0644)
			if err != nil {
				return err
			}
		}
	}

	return nil
}