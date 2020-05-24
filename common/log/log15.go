package log

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/inconshreveable/log15"
	"github.com/pkg/errors"
)

type Logger log15.Logger

var logger Logger
var errorFile string

func init() {
	filePath := path.Join(GetCurrentPath(), "logs")
	errorFile = path.Join(filePath, "error.json")
	if _, err := CreateDirIfMissing(filePath); err != nil {
		panic(err)
	}
	logger = New("hotstuff", "logger")
}

func New(ctx ...interface{}) Logger {
	log := log15.New(ctx...)
	log.SetHandler(log15.SyncHandler(log15.MultiHandler(
		log15.StreamHandler(os.Stderr, log15.LogfmtFormat()),
		log15.LvlFilterHandler(log15.LvlError, log15.Must.FileHandler(
			errorFile, log15.JsonFormat())),
	)))

	return log
}

func GetLogger(ctx ...interface{}) Logger {
	if len(ctx) == 0 {
		return logger
	}
	return New(ctx...)
}

func GetCurrentPath() string {
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		panic(err)
	}
	return strings.Replace(dir, "\\", "/", -1)
}

// CreateDirIfMissing creates a dir for dirPath if not already exists. If the dir is empty it returns true
func CreateDirIfMissing(dirPath string) (bool, error) {
	// if dirPath does not end with a path separator, it leaves out the last segment while creating directories
	if !strings.HasSuffix(dirPath, "/") {
		dirPath = dirPath + "/"
	}
	err := os.MkdirAll(path.Dir(dirPath), 0755)
	if err != nil {
		return false, errors.Wrapf(err, "error creating dir [%s]", dirPath)
	}
	return DirEmpty(dirPath)
}

// DirEmpty returns true if the dir at dirPath is empty
func DirEmpty(dirPath string) (bool, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return false, errors.Wrapf(err, "error opening dir [%s]", dirPath)
	}
	defer func() {
		if err := f.Close(); err != nil {
			panic(err)
		}
	}()

	_, err = f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	err = errors.Wrapf(err, "error checking if dir [%s] is empty", dirPath)
	return false, err
}
