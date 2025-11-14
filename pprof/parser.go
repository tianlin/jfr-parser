package pprof

import (
	"fmt"
	"io"
	"strconv"

	"github.com/grafana/jfr-parser/parser"
)

type pprofOptions struct {
	truncatedFrame       bool
	disablePanicRecovery bool
}
type Option func(*pprofOptions)

func WithTruncatedFrame(v bool) Option {
	return func(o *pprofOptions) {
		o.truncatedFrame = v
	}
}

func WithDisablePanicRecovery(v bool) Option {
	return func(o *pprofOptions) {
		o.disablePanicRecovery = v
	}
}

func ParseJFR(body []byte, pi *ParseInput, jfrLabels *LabelsSnapshot, opts ...Option) (res *Profiles, err error) {
	o := &pprofOptions{
		truncatedFrame:       false,
		disablePanicRecovery: false,
	}
	for i := range opts {
		opts[i](o)
	}

	if !o.disablePanicRecovery {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("jfr parser panic: %v", r)
			}
		}()
	}

	p := parser.NewParser(body, parser.Options{
		SymbolProcessor: parser.ProcessSymbols,
	})
	return parse(p, pi, jfrLabels, o)
}

func parse(parser *parser.Parser, piOriginal *ParseInput, jfrLabels *LabelsSnapshot, opt *pprofOptions) (result *Profiles, err error) {
	var event string

	builders := newJfrPprofBuilders(parser, jfrLabels, piOriginal, opt)
	stringIndex := map[string]int64{}
	for k, v := range jfrLabels.Strings {
		stringIndex[v] = k
	}

	var values = [2]int64{1, 0}

	for {
		typ, err := parser.ParseEvent()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("jfr parser ParseEvent error: %w", err)
		}

		switch typ {
		case parser.TypeMap.T_EXECUTION_SAMPLE:
			ctxId := int64(parser.ExecutionSample.ContextId)
			ctx := builders.contextLabels(uint64(ctxId))
			if ctx == nil {
				ctx = &Context{
					Labels: make(map[int64]int64),
				}
				if builders.jfrLabels.Contexts == nil {
					builders.jfrLabels.Contexts = make(map[int64]*Context)
				}
				builders.jfrLabels.Contexts[ctxId] = ctx
			}

			var hasThreadInfo bool
			for kIndex := range ctx.Labels {
				if builders.jfrLabels.Strings[kIndex] == "thread_id" {
					hasThreadInfo = true
				}
			}

			if !hasThreadInfo {
				ti := parser.GetThreadInfo(parser.ExecutionSample.SampledThread)
				if ti != nil {
					k := addString(builders.jfrLabels, stringIndex, "thread_id")
					v := addString(builders.jfrLabels, stringIndex,
						strconv.FormatInt(int64(ti.OsThreadId), 10))
					ctx.Labels[k] = v
					k = addString(builders.jfrLabels, stringIndex, "thread_name")
					v = addString(builders.jfrLabels, stringIndex, ti.OsName)
					ctx.Labels[k] = v
				}
			}

			ts := parser.GetThreadState(parser.ExecutionSample.State)
			correlation := StacktraceCorrelation{
				ContextId: parser.ExecutionSample.ContextId,
				SpanId:    parser.ExecutionSample.SpanId,
				SpanName:  parser.ExecutionSample.SpanName,
			}
			if ts != nil && ts.Name != "STATE_SLEEPING" {
				builders.addStacktrace(sampleTypeCPU, correlation, parser.ExecutionSample.StackTrace, values[:1])
			}
			if event == "wall" {
				builders.addStacktrace(sampleTypeWall, correlation, parser.ExecutionSample.StackTrace, values[:1])
			}
			if ctxId == 0 {
				delete(builders.jfrLabels.Contexts, ctxId)
			}
		case parser.TypeMap.T_WALL_CLOCK_SAMPLE:
			values[0] = int64(parser.WallClockSample.Samples)
			builders.addStacktrace(sampleTypeWall, StacktraceCorrelation{}, parser.WallClockSample.StackTrace, values[:1])
		case parser.TypeMap.T_ALLOC_IN_NEW_TLAB:
			values[1] = int64(parser.ObjectAllocationInNewTLAB.TlabSize)
			correlation := StacktraceCorrelation{
				ContextId: parser.ObjectAllocationInNewTLAB.ContextId,
				SpanId:    parser.ObjectAllocationInNewTLAB.SpanId,
				SpanName:  parser.ObjectAllocationInNewTLAB.SpanName,
			}
			builders.addStacktrace(sampleTypeInTLAB, correlation, parser.ObjectAllocationInNewTLAB.StackTrace, values[:2])
		case parser.TypeMap.T_ALLOC_OUTSIDE_TLAB:
			values[1] = int64(parser.ObjectAllocationOutsideTLAB.AllocationSize)
			correlation := StacktraceCorrelation{
				ContextId: parser.ObjectAllocationOutsideTLAB.ContextId,
				SpanId:    parser.ObjectAllocationOutsideTLAB.SpanId,
				SpanName:  parser.ObjectAllocationOutsideTLAB.SpanName,
			}
			builders.addStacktrace(sampleTypeOutTLAB, correlation, parser.ObjectAllocationOutsideTLAB.StackTrace, values[:2])
		case parser.TypeMap.T_ALLOC_SAMPLE:
			values[1] = int64(parser.ObjectAllocationSample.Weight)
			builders.addStacktrace(sampleTypeAllocSample, StacktraceCorrelation{}, parser.ObjectAllocationSample.StackTrace, values[:2])
		case parser.TypeMap.T_MONITOR_ENTER:
			values[1] = int64(parser.JavaMonitorEnter.Duration)
			correlation := StacktraceCorrelation{
				ContextId: parser.JavaMonitorEnter.ContextId,
				SpanId:    parser.JavaMonitorEnter.SpanId,
				SpanName:  parser.JavaMonitorEnter.SpanName,
			}
			builders.addStacktrace(sampleTypeLock, correlation, parser.JavaMonitorEnter.StackTrace, values[:2])
		case parser.TypeMap.T_THREAD_PARK:
			values[1] = int64(parser.ThreadPark.Duration)
			builders.addStacktrace(sampleTypeThreadPark, StacktraceCorrelation{}, parser.ThreadPark.StackTrace, values[:2])
		case parser.TypeMap.T_LIVE_OBJECT:
			builders.addStacktrace(sampleTypeLiveObject, StacktraceCorrelation{}, parser.LiveObject.StackTrace, values[:1])
		case parser.TypeMap.T_MALLOC:
			values[1] = int64(parser.Malloc.Size)
			builders.addStacktrace(sampleTypeMalloc, StacktraceCorrelation{}, parser.Malloc.StackTrace, values[:2])
		case parser.TypeMap.T_ACTIVE_SETTING:
			if parser.ActiveSetting.Name == "event" {
				event = parser.ActiveSetting.Value
			}

		}
	}

	result = builders.build(event)

	return result, nil
}

func addString(jfrLabels *LabelsSnapshot, stringIndex map[string]int64, s string) int64 {
	if i, ok := stringIndex[s]; ok {
		return i
	}

	if jfrLabels.Strings == nil {
		jfrLabels.Strings = make(map[int64]string)
	}

	i := int64(len(stringIndex) + 1)
	jfrLabels.Strings[i] = s
	stringIndex[s] = i
	return i
}
