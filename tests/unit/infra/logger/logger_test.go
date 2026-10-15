//go:build unit

package logger_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/andreis3/isura-ledger-ms/internal/infra/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/trace"
)

var _ = Describe("INTERNAL :: INFRA :: LOGGER :: LOGGER", func() {
	Describe("#NewLogger", func() {
		Context("success cases", func() {
			It("should write JSON and text levels and enrich valid trace contexts", func() {
				// Arrange.
				stdoutRead, stdoutWrite, err := os.Pipe()
				Expect(err).NotTo(HaveOccurred())
				stderrRead, stderrWrite, err := os.Pipe()
				Expect(err).NotTo(HaveOccurred())
				oldStdout, oldStderr := os.Stdout, os.Stderr
				oldDefault := slog.Default()
				oldEnv, hadEnv := os.LookupEnv("ENV")
				os.Stdout, os.Stderr = stdoutWrite, stderrWrite
				Expect(os.Setenv("ENV", "development")).To(Succeed())
				DeferCleanup(func() {
					os.Stdout, os.Stderr = oldStdout, oldStderr
					slog.SetDefault(oldDefault)
					if hadEnv {
						Expect(os.Setenv("ENV", oldEnv)).To(Succeed())
					} else {
						Expect(os.Unsetenv("ENV")).To(Succeed())
					}
					_ = stdoutWrite.Close()
					_ = stderrWrite.Close()
				})
				sut := logger.NewLogger()

				// Act.
				sut.DebugJSON("json debug")
				sut.InfoJSON("json info")
				sut.WarnJSON("json warning")
				sut.ErrorJSON("json error")
				sut.CriticalJSON("json critical")
				sut.DebugText("text debug")
				sut.InfoText("text info")
				sut.WarnText("text warning")
				sut.ErrorText("text error")
				sut.CriticalText("text critical")
				sut.SlogJSON().Info("direct json")
				sut.SlogText().Info("direct text")
				traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
				Expect(err).NotTo(HaveOccurred())
				spanID, err := trace.SpanIDFromHex("0102030405060708")
				Expect(err).NotTo(HaveOccurred())
				ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled}))
				enriched := sut.WithTrace(ctx)
				enriched.Info("trace event")
				withoutTrace := sut.WithTrace(context.Background())
				withoutTrace.Info("plain event")
				Expect(enriched).NotTo(BeNil())
				Expect(withoutTrace).To(BeIdenticalTo(sut.SlogJSON()))

				// Assert.
				Expect(stdoutWrite.Close()).To(Succeed())
				Expect(stderrWrite.Close()).To(Succeed())
				jsonOutput, err := io.ReadAll(stdoutRead)
				Expect(err).NotTo(HaveOccurred())
				textOutput, err := io.ReadAll(stderrRead)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(jsonOutput)).To(ContainSubstring("CRITICAL"))
				Expect(string(jsonOutput)).To(ContainSubstring(traceID.String()))
				Expect(string(jsonOutput)).To(ContainSubstring("plain event"))
				Expect(strings.ToLower(string(textOutput))).To(ContainSubstring("text critical"))
			})
		})
	})
})
