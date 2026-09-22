package zlog

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/natefinch/lumberjack"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/metadata"

	"va_visionai_server/conf"
	"va_visionai_server/internal/constants"
	vai "va_visionai_server/internal/va_interface"
)

var Logger *zap.Logger

type Option struct {
	ServiceName string
	LogPath     string
	LogLevel    string
	MaxSize     int
	MaxAgeDays  int
	MaxBackups  int
	Compress    bool
}

func getLogOption() Option {
	config := conf.GlobalConfig.LogConfig
	hostName, _ := os.Hostname()
	return Option{
		ServiceName: string(conf.GlobalConfig.ServerName),
		LogPath:     strings.Replace(config.LogPath, "__POD__", hostName, -1),
		LogLevel:    config.LogLevel,
		MaxSize:     config.MaxSize,
		MaxAgeDays:  config.MaxAgeDays,
		MaxBackups:  config.MaxBackups,
		Compress:    config.Compress,
	}
}

func InitLogger() {
	option := getLogOption()
	var writeSyncer zapcore.WriteSyncer
	isProd := conf.IsProd()
	isDev := conf.IsDev()

	// Configure file logging using lumberjack
	hook := lumberjack.Logger{
		Filename:   option.LogPath,
		MaxSize:    option.MaxSize,
		MaxBackups: option.MaxBackups,
		MaxAge:     option.MaxAgeDays,
		Compress:   option.Compress,
	}

	// Set up appropriate write syncer based on environment
	if isProd {
		// Production only writes to file
		writeSyncer = zapcore.AddSync(&hook)
	} else if isDev {
		// Development writes to both file and stdout
		writeSyncer = zapcore.NewMultiWriteSyncer(
			zapcore.AddSync(&hook),
			zapcore.AddSync(os.Stdout),
		)
	} else if conf.IsLocal() {
		// Local only writes to stdout
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	// Configure log level
	var level zapcore.Level
	switch option.LogLevel {
	case "debug":
		level = zap.DebugLevel
	case "info":
		level = zap.InfoLevel
	case "error":
		level = zap.ErrorLevel
	default:
		level = zap.InfoLevel
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "linenum",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.FullCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}

	// Set up atomic level
	atomicLevel := zap.NewAtomicLevel()
	atomicLevel.SetLevel(level)

	// Configure encoder and core based on environment
	var encoder zapcore.Encoder
	if isProd || isDev {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	core := zapcore.NewCore(encoder, writeSyncer, level)

	// Initialize logger with development mode and service name
	development := zap.Development()
	hostName, _ := os.Hostname()
	filed := zap.Fields(zap.String("serviceName", option.ServiceName), zap.String("HostName", hostName))

	Logger = zap.New(core, filed, development)
}

func parseCtx(ctx context.Context) []zap.Field {
	zapFields := make([]zap.Field, 0)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		traceIDList := md.Get(constants.CtxTraceID)
		if len(traceIDList) > 0 {
			zapFields = append(zapFields, zap.String(constants.CtxTraceID, traceIDList[0]))
		}
	}
	if v := ctx.Value(constants.CtxUserID); v != nil {
		if userID, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String("UserID", userID))
		}
	}

	if v := ctx.Value(constants.CtxAppStore); v != nil {
		if appStore, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxAppStore, appStore))
		}
	}

	if v := ctx.Value(constants.CtxOSName); v != nil {
		if osName, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxOSName, osName))
		}
	}

	if v := ctx.Value(constants.CtxLang); v != nil {
		if lang, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxLang, lang))
		}
	}

	if v := ctx.Value(constants.CtxAppVersiopn); v != nil {
		if appVersion, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxAppVersiopn, appVersion))
		}
	}

	if v := ctx.Value(constants.CtxProjectID); v != nil {
		if projectID, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxProjectID, projectID))
		}
	}

	if v := ctx.Value(constants.CtxIP); v != nil {
		if ip, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxIP, ip))
		}
	}

	if v := ctx.Value(constants.CtxModelDeploymentName); v != nil {
		if modelDeploymentName, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxModelDeploymentName, modelDeploymentName))
		}
	}

	// if v := ctx.Value(constants.CtxModelID); v != nil {
	// 	zapFields = append(zapFields, zap.Any(constants.CtxModelID, v))
	// }

	if v := ctx.Value(constants.CtxModelName); v != nil {
		if modelName, ok := v.(string); ok {
			zapFields = append(zapFields, zap.String(constants.CtxModelName, modelName))
			modelID := vai.Model_value[modelName]
			zapFields = append(zapFields, zap.Int32(constants.CtxModelID, modelID))
		} else {
			zapFields = append(zapFields, zap.String(constants.CtxModelName, "unknown"))
			zapFields = append(zapFields, zap.Int32(constants.CtxModelID, 0))
		}
	}

	return zapFields
}

func getCurrentFunctionName() string {
	defer func() {
		if r := recover(); r != nil {
			Logger.Error("Recover: getCurrentFunctionName", zap.Any("error", r))
			return
		}
	}()
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	fn := runtime.FuncForPC(pc)
	return fn.Name()
}

func LogWithContext(ctx context.Context) *zap.Logger {
	if Logger == nil {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		Logger, _ = config.Build()
	}
	if ctx == nil {
		return Logger
	}
	fields := parseCtx(ctx)
	fields = append(fields, zap.String("Function", getCurrentFunctionName()))
	fields = append(fields, zap.Int64("Timestamp", time.Now().UnixMilli()))
	return Logger.With(fields...)
}
