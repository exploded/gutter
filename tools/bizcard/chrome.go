package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// findChrome looks for Chrome at the Windows default install, then $CHROME,
// then the usual macOS and Linux locations.
func findChrome() (string, error) {
	var tried []string
	exists := func(p string) bool {
		tried = append(tried, p)
		fi, err := os.Stat(p)
		return err == nil && !fi.IsDir()
	}
	if runtime.GOOS == "windows" {
		win := []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`}
		if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
			win = append(win, filepath.Join(pf, `Google\Chrome\Application\chrome.exe`))
		}
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			win = append(win, filepath.Join(la, `Google\Chrome\Application\chrome.exe`))
		}
		for _, p := range win {
			if exists(p) {
				return p, nil
			}
		}
	}
	if p := os.Getenv("CHROME"); p != "" {
		if exists(p) {
			return p, nil
		}
		if lp, err := exec.LookPath(p); err == nil {
			return lp, nil
		}
	}
	for _, p := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/snap/bin/chromium",
	} {
		if exists(p) {
			return p, nil
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome"} {
		if lp, err := exec.LookPath(name); err == nil {
			return lp, nil
		}
	}
	return "", fmt.Errorf("can't find Chrome (tried %s); set CHROME to its path", strings.Join(tried, ", "))
}

// headless runs one throwaway headless Chrome with its own profile, so it
// never touches (or opens a tab in) a Chrome the user already has running.
// The virtual-time budget lets the page's font loading and fit-and-check
// script finish before Chrome prints, screenshots or dumps the DOM.
func headless(chrome, workDir string, args ...string) ([]byte, error) {
	profile, err := os.MkdirTemp(workDir, "profile-*")
	if err != nil {
		return nil, err
	}
	base := []string{
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--user-data-dir=" + profile,
		"--allow-file-access-from-files",
		"--hide-scrollbars",
		"--virtual-time-budget=8000",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome, append(base, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil, errors.New("headless Chrome timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("headless Chrome: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return []byte(stdout.String()), nil
}

// screenshot captures w×h CSS px of the page at the given device scale.
func screenshot(chrome, workDir, pageURL, out string, w, h int, scale float64, extra ...string) error {
	args := append([]string{
		fmt.Sprintf("--window-size=%d,%d", w, h),
		fmt.Sprintf("--force-device-scale-factor=%g", scale),
		"--screenshot=" + out,
	}, extra...)
	if _, err := headless(chrome, workDir, append(args, pageURL)...); err != nil {
		return err
	}
	return mustExist(out)
}

// printPDF prints the page to a PDF using the page's own @page size.
func printPDF(chrome, workDir, pageURL, out string) error {
	if _, err := headless(chrome, workDir, "--no-pdf-header-footer", "--print-to-pdf="+out, pageURL); err != nil {
		return err
	}
	return mustExist(out)
}

var checkAttr = regexp.MustCompile(`<html[^>]*\sdata-check="([^"]*)"`)

// layoutCheck loads the page, lets its fit-and-check script run, and returns
// what the script reported in <html data-check>: nil when it says "ok".
func layoutCheck(chrome, workDir, pageURL string) error {
	dom, err := headless(chrome, workDir, "--dump-dom", pageURL)
	if err != nil {
		return err
	}
	m := checkAttr.FindSubmatch(dom)
	if m == nil {
		return errors.New("the layout check didn't run (no data-check on <html>); is JavaScript or font loading failing?")
	}
	if got := html.UnescapeString(string(m[1])); got != "ok" {
		return errors.New(strings.ReplaceAll(got, " | ", "\n  "))
	}
	return nil
}

func mustExist(path string) error {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return fmt.Errorf("Chrome didn't write %s", path)
	}
	return nil
}
