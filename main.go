package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"github.com/uptrace/uptrace-go/uptrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"os"
	"reflect"
	"unsafe"
)

// Wrap zap logger to extend Zap with API that accepts a context.Context.
var log *otelzap.Logger

func initLogger() {

	config := zap.NewDevelopmentConfig()

	// Change the log level to Debug
	config.Level.SetLevel(zap.DebugLevel)
	config.Sampling = nil

	log = otelzap.New(zap.Must(config.Build()), func(l *otelzap.Logger) {
		// Get a Value representing the struct
		v := reflect.ValueOf(l).Elem()

		// Get a Value representing the unexported field
		f := v.FieldByName("minLevel")

		if !f.CanInt() {
			panic("Cannot set unexported field minLevel on logger")
		}

		// Ensure the field is settable
		if !f.CanSet() {
			// If not, make it addressable (and thus settable)
			f = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
		}

		// Set the unexported field
		f.SetInt(int64(zapcore.InfoLevel))

		fmt.Println(l)
	})
	otelzap.ReplaceGlobals(log)
}

func initUptrace() {

	fmt.Println(log.Level())
	if _, exits := os.LookupEnv("UPTRACE_DSN"); !exits {
		fmt.Println("warn: UPTRACE_DSN not set")
		os.Exit(1)
	}
	uptrace.ConfigureOpentelemetry(
		// copy your project DSN here or use UPTRACE_DSN env var
		uptrace.WithServiceName("otlp-example-uptrace"),
		uptrace.WithServiceVersion("v1.0.0"),
		uptrace.WithDeploymentEnvironment("production"),
		uptrace.WithResourceDetectors(),
	)
}

const (
	instrumentationName = "main"
)

var tracer = otel.Tracer(instrumentationName)

func add(ctx context.Context, x, y int64) int64 {
	ctx, span := tracer.Start(ctx, "Addition")
	defer span.End()

	return x + y
}

func multiply(ctx context.Context, x, y int64) int64 {
	ctx, span := tracer.Start(ctx, "Multiplication", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	log.Ctx(ctx).Info("multiply ", zap.Int64("x", x), zap.Int64("y", y))

	return x * y
}

func getDatabaseUser(ctx context.Context, email string) int64 {
	var span trace.Span
	ctx, span = tracer.Start(ctx, "getDatabaseUser", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()
	sugar := log.Sugar()

	sugar.Ctx(ctx).Infow("getDatabaseUser INFO ", zap.String("email", email))
	log.InfoContext(ctx, "getDatabaseUser INFO2 ", zap.String("email", email))
	log.Ctx(ctx).Error("getDatabaseUser ERR ", zap.String("email", email))
	return 42
}

func getUser(ctx context.Context, email string) int64 {
	ctx, span := tracer.Start(ctx, "getUser", trace.WithSpanKind(trace.SpanKindServer))

	defer span.End()
	return getDatabaseUser(ctx, email)
}
func someFuncWithError(ctx context.Context) {
	ctx, span := tracer.Start(ctx, "someFuncWithError")
	defer span.End()
	err := errors.New("dummy error")
	span.RecordError(err, trace.WithStackTrace(true))
	span.SetStatus(codes.Error, err.Error())
}

func main() {
	initLogger()
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		fmt.Println("otel - error: ", err)
	}))
	ctx := context.Background()
	initUptrace()

	// Send buffered spans and free resources.
	defer uptrace.Shutdown(ctx)

	ctx, span := tracer.Start(ctx, "main")
	defer span.End()
	defer log.Sync()

	// And then pass ctx to propagate the span.
	log.Ctx(ctx).Error("hello from zap",
		zap.Error(errors.New("hello world")),
		zap.String("foo", "bar"))

	log.Ctx(ctx).Info("This is an INFO")
	log.Ctx(ctx).Warn("This is a WARN")
	log.Ctx(ctx).Error("This is an ERROR")

	log.Ctx(ctx).Info("starting the application")
	log.Ctx(ctx).Info("getting the user ", zap.Int64("getUser", getUser(ctx, "haha@bla.com")))
	log.Ctx(ctx).Info("the answer is ", zap.Int64("multi result", add(ctx, multiply(ctx, multiply(ctx, 2, 2), 10), 2)))
	someFuncWithError(ctx)

	fmt.Printf("trace: %s\n", uptrace.TraceURL(span))
}
