package log

import (
	"log"
	"path/filepath"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

func init() {
	Log = logrus.New()
	Log.SetLevel(logrus.DebugLevel)
	Log.Hooks.Add(newLfsHook(31))
	logrus.SetOutput(log.Writer())
}

func newLfsHook(maxRemainCnt uint) logrus.Hook {
	logName := filepath.Join("logs", "log")
	writer, err := rotatelogs.New(
		logName+"%Y%m%d.log",
		rotatelogs.WithRotationTime(time.Hour*24),
		rotatelogs.WithRotationCount(maxRemainCnt),
	)

	if err != nil {
		Log.Errorf("config local file system for logger error: %v", err)
	}

	lfsHook := lfshook.NewHook(lfshook.WriterMap{
		logrus.DebugLevel: writer,
		logrus.InfoLevel:  writer,
		logrus.WarnLevel:  writer,
		logrus.ErrorLevel: writer,
		logrus.FatalLevel: writer,
		logrus.PanicLevel: writer,
	}, &logrus.TextFormatter{DisableColors: false})

	return lfsHook
}
