package log

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xtls/xray-core/common/platform"
	"github.com/xtls/xray-core/common/signal/done"
	"github.com/xtls/xray-core/common/signal/semaphore"
)

// Writer is the interface for writing logs.
type Writer interface {
	Write(string) error
	io.Closer
}

// WriterCreator is a function to create LogWriters.
type WriterCreator func() Writer

type generalLogger struct {
	creator WriterCreator
	buffer  chan Message
	access  *semaphore.Instance
	done    *done.Instance
}

type serverityLogger struct {
	inner    *generalLogger
	logLevel Severity
}

// NewLogger returns a generic log handler that can handle all type of messages.
func NewLogger(logWriterCreator WriterCreator) Handler {
	return &generalLogger{
		creator: logWriterCreator,
		buffer:  make(chan Message, 128),
		access:  semaphore.New(1),
		done:    done.New(),
	}
}

func ReplaceWithSeverityLogger(serverity Severity) {
	w := CreateStdoutLogWriter()
	g := &generalLogger{
		creator: w,
		buffer:  make(chan Message, 128),
		access:  semaphore.New(1),
		done:    done.New(),
	}
	s := &serverityLogger{
		inner:    g,
		logLevel: serverity,
	}
	RegisterHandler(s)
}

func (l *serverityLogger) Handle(msg Message) {
	switch msg := msg.(type) {
	case *GeneralMessage:
		if msg.Severity <= l.logLevel {
			l.inner.Handle(msg)
		}
	default:
		l.inner.Handle(msg)
	}
}

func (l *generalLogger) run() {
	defer l.access.Signal()

	dataWritten := false
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	logger := l.creator()
	if logger == nil {
		return
	}
	defer logger.Close()

	for {
		select {
		case <-l.done.Wait():
			return
		case msg := <-l.buffer:
			logger.Write(msg.String() + platform.LineSeparator())
			dataWritten = true
		case <-ticker.C:
			if !dataWritten {
				return
			}
			dataWritten = false
		}
	}
}

func (l *generalLogger) Handle(msg Message) {
	select {
	case l.buffer <- msg:
	default:
	}

	select {
	case <-l.access.Wait():
		go l.run()
	default:
	}
}

func (l *generalLogger) Close() error {
	return l.done.Close()
}

type consoleLogWriter struct {
	out    io.Writer
	colored bool
}

func (w *consoleLogWriter) Write(s string) error {
	if w.colored {
		s = colorize(s)
	}
	_, err := io.WriteString(w.out, time.Now().Format("2006-01-02T15:04:05.000Z07:00")+" "+s)
	return err
}

func (w *consoleLogWriter) Close() error {
	return nil
}

func colorize(s string) string {
	for _, c := range [][2]string{
		{"[Error] ", "\033[31m"},
		{"[Warning] ", "\033[33m"},
		{"[Info] ", "\033[32m"},
		{"[Debug] ", "\033[90m"},
	} {
		if strings.HasPrefix(s, c[0]) {
			return c[1] + c[0] + "\033[0m" + s[len(c[0]):]
		}
	}
	return s
}

type fileLogWriter struct {
	file       *os.File
	path       string
	bytes      int64
	maxSize    int64
	maxBackups int
	maxAge     time.Duration
}

// ponytail: rotation on write if file exceeds maxSize. Keeps maxBackups numbered backups (.1, .2, ...).
// Time-based cleanup deletes backups older than maxAge when rotate is called.
func (w *fileLogWriter) Write(s string) error {
	ts := time.Now().Format("2006-01-02T15:04:05.000Z07:00")
	line := ts + " " + s
	if w.bytes+int64(len(line)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return err
		}
	}
	n, err := io.WriteString(w.file, line)
	w.bytes += int64(n)
	return err
}

func (w *fileLogWriter) rotate() error {
	w.file.Close()
	for i := w.maxBackups; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", w.path, i)
		os.Remove(old)
		if i > 1 {
			os.Rename(fmt.Sprintf("%s.%d", w.path, i-1), old)
		}
	}
	os.Rename(w.path, w.path+".1")
	file, err := os.OpenFile(w.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	w.file = file
	w.bytes = 0
	if w.maxAge > 0 {
		matches, _ := filepath.Glob(w.path + ".*")
		cutoff := time.Now().Add(-w.maxAge)
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.ModTime().Before(cutoff) {
				os.Remove(m)
			}
		}
	}
	return nil
}

func (w *fileLogWriter) Close() error {
	return w.file.Close()
}

// CreateStdoutLogWriter returns a LogWriterCreator that creates LogWriter for stdout.
func CreateStdoutLogWriter() WriterCreator {
	return func() Writer {
		return &consoleLogWriter{out: os.Stdout, colored: true}
	}
}

// CreateStderrLogWriter returns a LogWriterCreator that creates LogWriter for stderr.
func CreateStderrLogWriter() WriterCreator {
	return func() Writer {
		return &consoleLogWriter{out: os.Stderr, colored: true}
	}
}

// CreateFileLogWriter returns a WriterCreator that creates file log writers.
// maxKeepDays: 0 disables time-based cleanup.
func CreateFileLogWriter(path string, maxKeepDays int) (WriterCreator, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	fi, _ := file.Stat()
	file.Close()
	var maxAge time.Duration
	if maxKeepDays > 0 {
		maxAge = time.Duration(maxKeepDays) * 24 * time.Hour
	}
	return func() Writer {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			return nil
		}
		var size int64
		if fi != nil {
			size = fi.Size()
		}
		return &fileLogWriter{
			file:       file,
			path:       path,
			bytes:      size,
			maxSize:    10 * 1024 * 1024,
			maxBackups: 3,
			maxAge:     maxAge,
		}
	}, nil
}

func init() {
	RegisterHandler(NewLogger(CreateStdoutLogWriter()))
}
