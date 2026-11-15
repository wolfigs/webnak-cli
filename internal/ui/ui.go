// Package ui is the CLI's terminal side: output for people or for machines,
// prompts, the browser, and a spinner.
package ui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// Out decides how things are printed.
type Out struct {
	JSON    bool // machine-readable output on stdout
	NoInput bool // never prompt; fail with a clear message instead
	Quiet   bool
	W       io.Writer
	E       io.Writer
}

func New() *Out { return &Out{W: os.Stdout, E: os.Stderr} }

// Interactive is whether prompts are allowed.
func (o *Out) Interactive() bool {
	return !o.NoInput && !o.JSON && term.IsTerminal(int(os.Stdin.Fd()))
}

// Fancy is whether colour and spinners suit stderr.
func (o *Out) Fancy() bool {
	return !o.JSON && os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(os.Stderr.Fd()))
}

func (o *Out) color(code, s string) string {
	if !o.Fancy() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (o *Out) Bold(s string) string  { return o.color("1", s) }
func (o *Out) Dim(s string) string   { return o.color("2", s) }
func (o *Out) Green(s string) string { return o.color("32", s) }
func (o *Out) Red(s string) string   { return o.color("31", s) }
func (o *Out) Cyan(s string) string  { return o.color("36", s) }
func (o *Out) Orange(s string) string {
	return o.color("38;5;208", s)
}

// Print writes a human line to stdout (skipped in JSON mode).
func (o *Out) Print(format string, a ...any) {
	if o.JSON || o.Quiet {
		return
	}
	fmt.Fprintf(o.W, format+"\n", a...)
}

// Info writes progress to stderr, so stdout stays clean for piping.
func (o *Out) Info(format string, a ...any) {
	if o.Quiet {
		return
	}
	fmt.Fprintf(o.E, format+"\n", a...)
}

// Success prints a tick line to stderr.
func (o *Out) Success(format string, a ...any) {
	o.Info("%s %s", o.Green("✓"), fmt.Sprintf(format, a...))
}

// Result prints v as JSON in JSON mode; otherwise calls human.
func (o *Out) Result(v any, human func()) {
	if o.JSON {
		enc := json.NewEncoder(o.W)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
		return
	}
	if human != nil {
		human()
	}
}

// Table prints aligned columns.
func (o *Out) Table(head []string, rows [][]string) {
	if o.JSON {
		return
	}
	w := make([]int, len(head))
	for i, h := range head {
		w[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(w) && len([]rune(c)) > w[i] {
				w[i] = len([]rune(c))
			}
		}
	}
	line := func(cells []string, style func(string) string) {
		var b strings.Builder
		for i, c := range cells {
			pad := w[i] - len([]rune(c))
			if i < len(cells)-1 {
				c += strings.Repeat(" ", pad+2)
			}
			b.WriteString(style(c))
		}
		fmt.Fprintln(o.W, strings.TrimRight(b.String(), " "))
	}
	line(head, o.Dim)
	for _, r := range rows {
		line(r, func(s string) string { return s })
	}
}

// ErrNoInput is returned when a prompt is needed but prompts are off.
var ErrNoInput = errors.New("this needs an answer, but prompts are off (use the flags shown in --help)")

var stdin = bufio.NewReader(os.Stdin)

// Ask prompts for a line of text.
func (o *Out) Ask(label, def string) (string, error) {
	if !o.Interactive() {
		if def != "" {
			return def, nil
		}
		return "", fmt.Errorf("%w: %s", ErrNoInput, label)
	}
	if def != "" {
		fmt.Fprintf(o.E, "%s %s ", o.Bold(label), o.Dim("("+def+")"))
	} else {
		fmt.Fprintf(o.E, "%s ", o.Bold(label))
	}
	s, err := stdin.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		s = def
	}
	return s, nil
}

// Secret prompts without echo.
func (o *Out) Secret(label string) (string, error) {
	if !o.Interactive() {
		return "", fmt.Errorf("%w: %s", ErrNoInput, label)
	}
	fmt.Fprintf(o.E, "%s ", o.Bold(label))
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(o.E)
	return strings.TrimSpace(string(b)), err
}

// Confirm asks yes/no; with prompts off it returns def.
func (o *Out) Confirm(label string, def bool) bool {
	if !o.Interactive() {
		return def
	}
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	s, err := o.Ask(label+" "+o.Dim("["+hint+"]"), "")
	if err != nil {
		return def
	}
	switch strings.ToLower(s) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	}
	return def
}

// Choose asks the user to pick one of options by number.
func (o *Out) Choose(label string, options []string) (int, error) {
	if !o.Interactive() {
		return -1, fmt.Errorf("%w: %s", ErrNoInput, label)
	}
	fmt.Fprintln(o.E, o.Bold(label))
	for i, op := range options {
		fmt.Fprintf(o.E, "  %s %s\n", o.Cyan(fmt.Sprintf("%2d)", i+1)), op)
	}
	for {
		s, err := o.Ask("Number:", "")
		if err != nil {
			return -1, err
		}
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil && n >= 1 && n <= len(options) {
			return n - 1, nil
		}
		fmt.Fprintln(o.E, o.Red("Pick a number from the list."))
	}
}

// OpenBrowser opens a URL; it returns false if it could not.
func OpenBrowser(url string) bool {
	if os.Getenv("WEBNAK_NO_BROWSER") != "" {
		return false
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Start() == nil
}

// Spinner shows progress on stderr when it is a terminal.
type Spinner struct {
	o    *Out
	mu   sync.Mutex
	msg  string
	stop chan struct{}
	done chan struct{}
}

func (o *Out) Spin(msg string) *Spinner {
	s := &Spinner{o: o, msg: msg, stop: make(chan struct{}), done: make(chan struct{})}
	if !o.Fancy() || o.Quiet {
		o.Info("%s", msg)
		close(s.done)
		return s
	}
	go func() {
		defer close(s.done)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		t := time.NewTicker(90 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			s.mu.Lock()
			m := s.msg
			s.mu.Unlock()
			fmt.Fprintf(o.E, "\r\x1b[2K%s %s", o.Orange(frames[i%len(frames)]), m)
			select {
			case <-s.stop:
				fmt.Fprint(o.E, "\r\x1b[2K")
				return
			case <-t.C:
			}
		}
	}()
	return s
}

// Update changes the message.
func (s *Spinner) Update(msg string) {
	s.mu.Lock()
	changed := s.msg != msg
	s.msg = msg
	s.mu.Unlock()
	if changed && (!s.o.Fancy() || s.o.Quiet) {
		s.o.Info("%s", msg)
	}
}

// Stop ends the spinner.
func (s *Spinner) Stop() {
	select {
	case <-s.done:
		return
	default:
	}
	close(s.stop)
	<-s.done
}
