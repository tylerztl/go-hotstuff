package log

import (
	"path"
	"testing"

	"github.com/inconshreveable/log15"
	"github.com/mattn/go-colorable"
)

func TestSetLogger(t *testing.T) {
	filePath := path.Join(GetCurrentPath(), "logs")
	errorFile := path.Join(filePath, "error.json")
	if _, err := CreateDirIfMissing(filePath); err != nil {
		t.Fatal(err)
	}

	log := log15.New("setLogger", "test")
	logLvl := log15.LvlDebug
	log.SetHandler(log15.SyncHandler(log15.MultiHandler(
		//log15.StreamHandler(os.Stderr, log15.LogfmtFormat()),
		log15.LvlFilterHandler(log15.LvlError, log15.Must.FileHandler(errorFile, log15.JsonFormat())),
		log15.LvlFilterHandler(logLvl, log15.StreamHandler(colorable.NewColorableStderr(), log15.TerminalFormat())),
	)))

	SetLogger(&DefaultLogger{Logger: log})
	GetLogger().Info("The logger are so cool!", "errorFilePath", errorFile)
}
