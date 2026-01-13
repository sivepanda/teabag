package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

const (
	configFile = ".config/teabag.conf"
)

type step int

const (
	stepFileBrowser step = iota
	stepAppImageDir
	stepAppName
	stepDescription
	stepIcon
	stepIconBrowser
	stepCategories
	stepProcessing
	stepComplete
	stepError
)

type installCompleteMsg struct {
	err error
}

type fileEntry struct {
	name  string
	path  string
	isDir bool
}

var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	titleStyle   = lipgloss.NewStyle().Bold(true)
)

type model struct {
	currentStep     step
	appImagePath    string
	appImageDir     string
	appName         string
	description     string
	iconPath        string
	categories      string
	input           string
	error           string
	message         string
	configPath      string
	firstTimeSetup  bool
	desktopFilePath string

	// File browser fields
	currentDir   string
	files        []fileEntry
	allFiles     []fileEntry // Unfiltered list for fuzzy search
	cursor       int
	searchMode   bool
	searchQuery  string
	fuzzyMatches []fuzzy.Match
}

func initialModel(appImagePath string) model {
	homeDir, _ := os.UserHomeDir()
	configPath := filepath.Join(homeDir, configFile)

	m := model{
		appImagePath: appImagePath,
		configPath:   configPath,
		categories:   "Utility;",
	}

	// If no app image path provided, start with file browser
	if appImagePath == "" {
		m.currentStep = stepFileBrowser
		m.currentDir, _ = os.Getwd()
		m.loadDirectory()
		return m
	}

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		m.firstTimeSetup = true
		m.currentStep = stepAppImageDir
		m.input = filepath.Join(homeDir, "Applications")
	} else {
		// Load existing config
		if dir, err := loadConfig(configPath); err == nil {
			m.appImageDir = dir
			m.currentStep = stepAppName
		} else {
			m.currentStep = stepError
			m.error = fmt.Sprintf("Failed to load config: %v", err)
		}
	}

	return m
}

func loadConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	// Parse APPIMAGE_DIR="path"
	for line := range strings.SplitSeq(string(data), "\n") {
		if dir, found := strings.CutPrefix(line, "APPIMAGE_DIR="); found {
			dir = strings.Trim(dir, "\"")
			return dir, nil
		}
	}

	return "", fmt.Errorf("APPIMAGE_DIR not found in config")
}

func saveConfig(path, appImageDir string) error {
	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}

	content := fmt.Sprintf("APPIMAGE_DIR=\"%s\"\n", appImageDir)
	return os.WriteFile(path, []byte(content), 0644)
}

// isImageFile checks if a filename has a common image extension
func isImageFile(name string) bool {
	lower := strings.ToLower(name)
	extensions := []string{".png", ".jpg", ".jpeg", ".svg", ".ico", ".xpm", ".bmp", ".gif", ".webp"}
	for _, ext := range extensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func (m *model) loadDirectory() {
	m.loadDirectoryWithFilter(false)
}

func (m *model) loadIconDirectory() {
	m.loadDirectoryWithFilter(true)
}

func (m *model) loadDirectoryWithFilter(iconMode bool) {
	m.allFiles = []fileEntry{}
	m.files = []fileEntry{}
	m.cursor = 0
	m.searchMode = false
	m.searchQuery = ""
	m.fuzzyMatches = nil

	entries, err := os.ReadDir(m.currentDir)
	if err != nil {
		m.error = fmt.Sprintf("Failed to read directory: %v", err)
		return
	}

	// Add parent directory entry if not at root
	if m.currentDir != "/" {
		m.allFiles = append(m.allFiles, fileEntry{
			name:  "..",
			path:  filepath.Dir(m.currentDir),
			isDir: true,
		})
	}

	// Add directories and matching files
	for _, entry := range entries {
		fullPath := filepath.Join(m.currentDir, entry.Name())

		if entry.IsDir() {
			m.allFiles = append(m.allFiles, fileEntry{
				name:  entry.Name(),
				path:  fullPath,
				isDir: true,
			})
		} else if iconMode && isImageFile(entry.Name()) {
			m.allFiles = append(m.allFiles, fileEntry{
				name:  entry.Name(),
				path:  fullPath,
				isDir: false,
			})
		} else if !iconMode && strings.HasSuffix(strings.ToLower(entry.Name()), ".appimage") {
			m.allFiles = append(m.allFiles, fileEntry{
				name:  entry.Name(),
				path:  fullPath,
				isDir: false,
			})
		}
	}

	m.files = m.allFiles
}

// fileEntrySource implements fuzzy.Source for file entries
type fileEntrySource []fileEntry

func (f fileEntrySource) String(i int) string {
	return f[i].name
}

func (f fileEntrySource) Len() int {
	return len(f)
}

func (m *model) applyFuzzySearch() {
	if m.searchQuery == "" {
		m.files = m.allFiles
		m.fuzzyMatches = nil
		m.cursor = 0
		return
	}

	matches := fuzzy.FindFrom(m.searchQuery, fileEntrySource(m.allFiles))
	m.fuzzyMatches = matches

	m.files = []fileEntry{}
	for _, match := range matches {
		m.files = append(m.files, m.allFiles[match.Index])
	}

	if m.cursor >= len(m.files) {
		m.cursor = max(0, len(m.files)-1)
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m *model) isFileBrowserStep() bool {
	return m.currentStep == stepFileBrowser || m.currentStep == stepIconBrowser
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle file browser steps (AppImage browser and Icon browser)
		if m.isFileBrowserStep() {
			return m.handleFileBrowserKeys(msg)
		}

		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "enter":
			newModel, cmd := m.handleEnter()
			return newModel, cmd

		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}

		default:
			if m.currentStep != stepProcessing && m.currentStep != stepComplete && m.currentStep != stepError {
				m.input += msg.String()
			}
		}

	case installCompleteMsg:
		if msg.err != nil {
			m.currentStep = stepError
			m.error = msg.err.Error()
		} else {
			m.currentStep = stepComplete
		}
		return m, nil
	}

	return m, nil
}

func (m model) handleFileBrowserKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// In search mode, handle typing
	if m.searchMode {
		switch key {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// Exit search mode
			m.searchMode = false
			m.searchQuery = ""
			m.files = m.allFiles
			m.fuzzyMatches = nil
			m.cursor = 0
			return m, nil
		case "enter":
			// Select current item and exit search mode
			m.searchMode = false
			return m.handleEnter()
		case "backspace":
			if len(m.searchQuery) > 0 {
				m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				m.applyFuzzySearch()
			}
			return m, nil
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down":
			if m.cursor < len(m.files)-1 {
				m.cursor++
			}
			return m, nil
		default:
			// Add character to search query
			if len(key) == 1 {
				m.searchQuery += key
				m.applyFuzzySearch()
			}
			return m, nil
		}
	}

	// Not in search mode
	switch key {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "/":
		// Enter search mode
		m.searchMode = true
		m.searchQuery = ""
		return m, nil

	case "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}

	case "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "j":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}

	case "enter":
		return m.handleEnter()

	case "backspace":
		// Go to parent directory
		if m.currentDir != "/" {
			m.currentDir = filepath.Dir(m.currentDir)
			if m.currentStep == stepIconBrowser {
				m.loadIconDirectory()
			} else {
				m.loadDirectory()
			}
		}
	}

	return m, nil
}

func (m model) handleEnter() (model, tea.Cmd) {
	switch m.currentStep {
	case stepFileBrowser:
		if len(m.files) == 0 {
			return m, nil
		}

		selected := m.files[m.cursor]
		if selected.isDir {
			// Navigate into directory
			m.currentDir = selected.path
			m.loadDirectory()
		} else {
			// AppImage file selected
			m.appImagePath = selected.path

			// Load config and proceed
			homeDir, _ := os.UserHomeDir()
			if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
				m.firstTimeSetup = true
				m.currentStep = stepAppImageDir
				m.input = filepath.Join(homeDir, "Applications")
			} else {
				if dir, err := loadConfig(m.configPath); err == nil {
					m.appImageDir = dir
					m.currentStep = stepAppName
				} else {
					m.currentStep = stepError
					m.error = fmt.Sprintf("Failed to load config: %v", err)
				}
			}
		}

	case stepAppImageDir:
		if m.input == "" {
			homeDir, _ := os.UserHomeDir()
			m.input = filepath.Join(homeDir, "Applications")
		}
		m.appImageDir = m.input

		// Create directory
		if err := os.MkdirAll(m.appImageDir, 0755); err != nil {
			m.currentStep = stepError
			m.error = fmt.Sprintf("Failed to create directory: %v", err)
			return m, nil
		}

		// Save config
		if err := saveConfig(m.configPath, m.appImageDir); err != nil {
			m.currentStep = stepError
			m.error = fmt.Sprintf("Failed to save config: %v", err)
			return m, nil
		}

		m.currentStep = stepAppName
		m.input = ""

	case stepAppName:
		if m.input == "" {
			m.error = "Application name is required"
			return m, nil
		}
		m.appName = m.input
		m.currentStep = stepDescription
		m.input = ""
		m.error = ""

	case stepDescription:
		m.description = m.input
		m.currentStep = stepIcon
		m.input = ""

	case stepIcon:
		// Check if user wants to browse for icon
		if m.input == "b" || m.input == "B" {
			homeDir, _ := os.UserHomeDir()
			m.currentDir = homeDir
			m.currentStep = stepIconBrowser
			m.input = ""
			m.loadIconDirectory()
			return m, nil
		}
		if m.input != "" {
			if _, err := os.Stat(m.input); os.IsNotExist(err) {
				m.error = fmt.Sprintf("Warning: Icon file not found: %s", m.input)
				return m, nil
			}
		}
		m.iconPath = m.input
		m.currentStep = stepCategories
		m.input = "Utility;"
		m.error = ""
		return m, nil

	case stepIconBrowser:
		if len(m.files) == 0 {
			return m, nil
		}

		selected := m.files[m.cursor]
		if selected.isDir {
			// Navigate into directory
			m.currentDir = selected.path
			m.loadIconDirectory()
		} else {
			// Image file selected
			m.iconPath = selected.path
			m.currentStep = stepCategories
			m.input = "Utility;"
			m.error = ""
		}
		return m, nil

	case stepCategories:
		if m.input == "" {
			m.input = "Utility;"
		}
		// Ensure categories end with semicolon
		if !strings.HasSuffix(m.input, ";") {
			m.input += ";"
		}
		m.categories = m.input
		m.currentStep = stepProcessing
		m.input = ""

		// Perform installation
		return m, m.install()

	case stepComplete, stepError:
		return m, tea.Quit
	}

	return m, nil
}

func (m *model) install() tea.Cmd {
	// Get absolute path
	absPath, err := filepath.Abs(m.appImagePath)
	if err != nil {
		return func() tea.Msg {
			return installCompleteMsg{err: fmt.Errorf("failed to get absolute path: %w", err)}
		}
	}

	appImageFilename := filepath.Base(absPath)
	destFile := filepath.Join(m.appImageDir, appImageFilename)

	// Move AppImage to central location
	if absPath != destFile {
		if err := os.Rename(absPath, destFile); err != nil {
			return func() tea.Msg {
				return installCompleteMsg{err: fmt.Errorf("failed to move file: %w", err)}
			}
		}
	}

	// Make executable
	if err := os.Chmod(destFile, 0755); err != nil {
		return func() tea.Msg {
			return installCompleteMsg{err: fmt.Errorf("failed to make executable: %w", err)}
		}
	}

	// Build desktop entry content
	desktopEntry := fmt.Sprintf("[Desktop Entry]\nName=%s\nExec=%s\nType=Application\nCategories=%s\n",
		m.appName, destFile, m.categories)

	if m.description != "" {
		desktopEntry += fmt.Sprintf("Comment=%s\n", m.description)
	}

	if m.iconPath != "" {
		desktopEntry += fmt.Sprintf("Icon=%s\n", m.iconPath)
	}

	// Create desktop entry in temp location first
	desktopFilename := strings.ToLower(strings.ReplaceAll(m.appName, " ", "-")) + ".desktop"
	tmpDesktopFile := filepath.Join(os.TempDir(), desktopFilename)

	if err := os.WriteFile(tmpDesktopFile, []byte(desktopEntry), 0644); err != nil {
		return func() tea.Msg {
			return installCompleteMsg{err: fmt.Errorf("failed to create temp desktop entry: %w", err)}
		}
	}

	desktopDir := "/usr/share/applications"
	m.desktopFilePath = filepath.Join(desktopDir, desktopFilename)

	// Try without permissions (run with sudo)
	cmd := exec.Command("cp", tmpDesktopFile, m.desktopFilePath)
	if errnoperm := cmd.Run(); errnoperm != nil {
		// Check if pkexec is available
		if _, err := exec.LookPath("pkexec"); err == nil {
			// Use pkexec (shows graphical prompt, doesn't interrupt TUI)
			return func() tea.Msg {
				// Copy desktop file
				cmd := exec.Command("pkexec", "cp", tmpDesktopFile, m.desktopFilePath)
				if err := cmd.Run(); err != nil {
					return installCompleteMsg{err: fmt.Errorf("failed to copy desktop file: %w", err)}
				}

				// Update desktop database
				if _, err := exec.LookPath("update-desktop-database"); err == nil {
					cmd := exec.Command("pkexec", "update-desktop-database", desktopDir)
					cmd.Run() // Ignore errors
				}

				m.message = fmt.Sprintf("Installation complete! %s should now appear in your application launcher", m.appName)
				return installCompleteMsg{err: nil}
			}
		}

	}
	// Permission denied, rerun with sudo
	return func() tea.Msg {
		return installCompleteMsg{err: fmt.Errorf("Installation requires administrator privileges. \n\n Rerun with sudo or pkexec.\n%w", err)}
	}
}

func (m model) View() string {
	var s strings.Builder

	s.WriteString(titleStyle.Render("Teabag - AppImage Installer") + "\n\n")

	switch m.currentStep {
	case stepFileBrowser:
		s.WriteString(fmt.Sprintf("Current directory: %s\n", m.currentDir))

		if m.searchMode {
			s.WriteString(fmt.Sprintf("Search: %s▌\n\n", m.searchQuery))
		} else {
			s.WriteString("\n")
		}

		if len(m.files) == 0 {
			if m.searchMode && m.searchQuery != "" {
				s.WriteString(infoStyle.Render("No matches found") + "\n")
			} else {
				s.WriteString(infoStyle.Render("No .appimage files or directories found") + "\n")
			}
		} else {
			for i, file := range m.files {
				cursor := " "
				if i == m.cursor {
					cursor = ">"
				}

				icon := "📄"
				if file.isDir {
					icon = "📁"
				}

				line := fmt.Sprintf("%s %s %s", cursor, icon, file.name)
				if i == m.cursor {
					line = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Render(line)
				}
				s.WriteString(line + "\n")
			}
		}

		if m.searchMode {
			s.WriteString("\n(Type to search, ↑/↓: navigate, Enter: select, Esc: cancel, Ctrl+C: quit)")
		} else {
			s.WriteString("\n(↑/↓ or j/k: navigate, /: search, Enter: select, Backspace: parent dir, Ctrl+C: quit)")
		}

	case stepAppImageDir:
		s.WriteString(infoStyle.Render("First-time setup: Configure AppImage storage location") + "\n\n")
		s.WriteString(fmt.Sprintf("Enter AppImage directory path: %s\n", m.input))
		s.WriteString("\n(Press Enter to use default, Ctrl+C to quit)")

	case stepAppName:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(fmt.Sprintf("Application name: %s\n", m.input))
		if m.error != "" {
			s.WriteString("\n" + errorStyle.Render("✗ "+m.error))
		}
		s.WriteString("\n(Press Enter to continue, Ctrl+C to quit)")

	case stepDescription:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(fmt.Sprintf("Description (optional): %s\n", m.input))
		s.WriteString("\n(Press Enter to continue, Ctrl+C to quit)")

	case stepIcon:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(fmt.Sprintf("Icon path (optional, type 'b' to browse): %s\n", m.input))
		if m.error != "" {
			s.WriteString("\n" + errorStyle.Render("✗ "+m.error))
		}
		s.WriteString("\n(Enter path, 'b' to browse, or Enter to skip)")

	case stepIconBrowser:
		s.WriteString(fmt.Sprintf("Browsing for icon - Current directory: %s\n", m.currentDir))

		if m.searchMode {
			s.WriteString(fmt.Sprintf("Search: %s▌\n\n", m.searchQuery))
		} else {
			s.WriteString("\n")
		}

		if len(m.files) == 0 {
			if m.searchMode && m.searchQuery != "" {
				s.WriteString(infoStyle.Render("No matches found") + "\n")
			} else {
				s.WriteString(infoStyle.Render("No image files or directories found") + "\n")
			}
		} else {
			for i, file := range m.files {
				cursor := " "
				if i == m.cursor {
					cursor = ">"
				}

				icon := "🖼️"
				if file.isDir {
					icon = "📁"
				}

				line := fmt.Sprintf("%s %s %s", cursor, icon, file.name)
				if i == m.cursor {
					line = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true).Render(line)
				}
				s.WriteString(line + "\n")
			}
		}

		if m.searchMode {
			s.WriteString("\n(Type to search, ↑/↓: navigate, Enter: select, Esc: cancel, Ctrl+C: quit)")
		} else {
			s.WriteString("\n(↑/↓ or j/k: navigate, /: search, Enter: select, Backspace: parent dir, Ctrl+C: quit)")
		}

	case stepCategories:
		s.WriteString(fmt.Sprintf("Installing: %s\n\n", filepath.Base(m.appImagePath)))
		s.WriteString(infoStyle.Render("Common categories: Utility, Development, Graphics, Network, Office, AudioVideo, Game") + "\n\n")
		s.WriteString(fmt.Sprintf("Categories (semicolon-separated): %s\n", m.input))
		s.WriteString("\n(Press Enter to install, Ctrl+C to quit)")

	case stepProcessing:
		s.WriteString(infoStyle.Render("➜ Processing...") + "\n")

	case stepComplete:
		s.WriteString(successStyle.Render("✓ Installation complete!") + "\n\n")
		s.WriteString(fmt.Sprintf("AppImage: %s\n", filepath.Join(m.appImageDir, filepath.Base(m.appImagePath))))
		s.WriteString(fmt.Sprintf("Desktop entry: %s\n\n", m.desktopFilePath))
		s.WriteString(m.message + "\n\n")
		s.WriteString("(Press any key to exit)")

	case stepError:
		s.WriteString(errorStyle.Render("✗ Error: "+m.error) + "\n\n")
		s.WriteString("(Press any key to exit)")
	}

	return s.String()
}

func main() {
	var appImagePath string

	if len(os.Args) >= 2 {
		appImagePath = os.Args[1]

		// Check if file exists
		if _, err := os.Stat(appImagePath); os.IsNotExist(err) {
			fmt.Println(errorStyle.Render(fmt.Sprintf("✗ File not found: %s", appImagePath)))
			os.Exit(1)
		}
	}

	p := tea.NewProgram(initialModel(appImagePath))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
