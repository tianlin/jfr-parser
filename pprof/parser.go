package pprof

import (
	"fmt"
	"github.com/grafana/jfr-parser/parser"
	"io"
	"strconv"
)

func ParseJFR(body []byte, pi *ParseInput, jfrLabels *LabelsSnapshot) (res *Profiles, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("jfr parser panic: %v", r)
		}
	}()
	p := parser.NewParser(body, parser.Options{
		SymbolProcessor: parser.ProcessSymbols,
	})
	return parse(p, pi, jfrLabels)
}

func parse(parser *parser.Parser, piOriginal *ParseInput, jfrLabels *LabelsSnapshot) (result *Profiles, err error) {
	var event string

	builders := newJfrPprofBuilders(parser, jfrLabels, piOriginal)

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
			ctx := builders.contextLabels(parser.ExecutionSample.ContextId)
			if ctx != nil {
				var hasThreadInfo bool
				for kIndex := range ctx.Labels {
					if builders.jfrLabels.Strings[kIndex] == "thread_id" {
						hasThreadInfo = true
					}
				}

				if !hasThreadInfo {
					ti := parser.GetThreadInfo(parser.ExecutionSample.SampledThread)
					if ti != nil {
						k := addString(builders.jfrLabels, "thread_id")
						v := addString(builders.jfrLabels,
							strconv.FormatInt(int64(ti.OsThreadId), 10))
						ctx.Labels[k] = v
						k = addString(builders.jfrLabels, "thread_name")
						v = addString(builders.jfrLabels, ti.OsName)
						ctx.Labels[k] = v
					}
				}
			}

			ts := parser.GetThreadState(parser.ExecutionSample.State)
			if ts != nil && ts.Name != "STATE_SLEEPING" {
				builders.addStacktrace(sampleTypeCPU, parser.ExecutionSample.ContextId, parser.ExecutionSample.StackTrace, values[:1])
			}
			if event == "wall" {
				builders.addStacktrace(sampleTypeWall, parser.ExecutionSample.ContextId, parser.ExecutionSample.StackTrace, values[:1])
			}
		case parser.TypeMap.T_ALLOC_IN_NEW_TLAB:
			values[1] = int64(parser.ObjectAllocationInNewTLAB.TlabSize)
			builders.addStacktrace(sampleTypeInTLAB, parser.ObjectAllocationInNewTLAB.ContextId, parser.ObjectAllocationInNewTLAB.StackTrace, values[:2])
		case parser.TypeMap.T_ALLOC_OUTSIDE_TLAB:
			values[1] = int64(parser.ObjectAllocationOutsideTLAB.AllocationSize)
			builders.addStacktrace(sampleTypeOutTLAB, parser.ObjectAllocationOutsideTLAB.ContextId, parser.ObjectAllocationOutsideTLAB.StackTrace, values[:2])
		case parser.TypeMap.T_MONITOR_ENTER:
			values[1] = int64(parser.JavaMonitorEnter.Duration)
			builders.addStacktrace(sampleTypeLock, parser.JavaMonitorEnter.ContextId, parser.JavaMonitorEnter.StackTrace, values[:2])
		case parser.TypeMap.T_THREAD_PARK:
			values[1] = int64(parser.ThreadPark.Duration)
			builders.addStacktrace(sampleTypeThreadPark, parser.ThreadPark.ContextId, parser.ThreadPark.StackTrace, values[:2])
		case parser.TypeMap.T_LIVE_OBJECT:
			builders.addStacktrace(sampleTypeLiveObject, 0, parser.LiveObject.StackTrace, values[:1])
		case parser.TypeMap.T_ACTIVE_SETTING:
			if parser.ActiveSetting.Name == "event" {
				event = parser.ActiveSetting.Value
			}

		}
	}

	result = builders.build(event)

	return result, nil
}

func addString(jfrLabels *LabelsSnapshot, s string) int64 {
	i := int64(len(jfrLabels.Strings)) + 1
	jfrLabels.Strings[i] = s
	return i
}
