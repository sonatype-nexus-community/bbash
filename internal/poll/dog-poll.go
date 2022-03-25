//
// Copyright (c) 2021-present Sonatype, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

//go:build go1.16
// +build go1.16

package poll

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"
	"github.com/sonatype-nexus-community/bbash/internal/db"
	"github.com/sonatype-nexus-community/bbash/internal/types"
	"go.uber.org/zap"
	"net/http"
	"os"
	"time"
)

var logger *zap.Logger

var dogApiClient IDogApiClient

func init() {
	dogApiClient = &DogApiClient{}
}

type IDogApiClient interface {
	getDDApiClient() (context.Context, *datadog.APIClient)
}

type DogApiClient struct {
}

var _ IDogApiClient = (*DogApiClient)(nil)

func (c *DogApiClient) getDDApiClient() (context.Context, *datadog.APIClient) {
	ctx := context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			// API Key
			"apiKeyAuth": {
				Key: os.Getenv("DD_CLIENT_API_KEY"),
			},
			// Application Key
			"appKeyAuth": {
				Key: os.Getenv("DD_CLIENT_APP_KEY"),
			},
		},
	)

	configuration := datadog.NewConfiguration()

	apiClient := datadog.NewAPIClient(configuration)
	return ctx, apiClient
}

const qryEnv = "env"
const qryEnvBaseTime = "envBaseTime"
const qryEnvExtraJsonFields = "envExtraJsonFields"
const qryFldFixedBugs = "fixed-bugs"

func pollTheDog(pollDB db.IDBPoll, now time.Time) (logs []ddLog, err error) {

	// get last poll time
	poll := pollDB.NewPoll()
	err = pollDB.SelectPoll(&poll)
	if err != nil {
		return
	}
	before := poll.LastPolled
	logger.Debug("before",
		zap.Time("before", before),
		zap.String("formattedBefore", before.Format(time.RFC3339)),
	)
	logger.Debug("now",
		zap.Time("now", before),
		zap.String("formattedNow", now.Format(time.RFC3339)),
	)

	pageCursor := ""
	isDone := false
	for err == nil && isDone == false {
		var logPage []ddLog
		isDone, pageCursor, logPage, err = fetchLogPage(before, now, &pageCursor)
		if err != nil {
			return
		}

		//logLen := len(logPage)
		//logFirst := ddLog{}
		//logLast := logFirst
		//if logLen > 0 {
		//	logFirst = logPage[0]
		//	logLast = logPage[logLen-1]
		//}
		//logger.Debug("log page",
		//	zap.Int("log count", len(logPage)),
		//	zap.Any("log first", logFirst),
		//	zap.Any("log last", logLast),
		//	zap.Bool("isDone", isDone),
		//	zap.String("pageCursor", pageCursor),
		//)
		logs = append(logs, logPage...)
	}

	logCount := len(logs)
	logger.Debug("total polled", zap.Int("log count", logCount))

	// UpdatePoll completed time
	poll.LastPolled = now
	if logCount > 0 {
		poll.EnvBaseTime = logs[logCount-1].Fields.envBaseTime
	}
	poll.LastPollCompleted = time.Now()
	err = pollDB.UpdatePoll(&poll)
	if err != nil {
		return
	}

	return
}

const maxLogsPerPage = 500

func fetchLogPage(before, now time.Time, pageCursor *string) (isDone bool, cursor string, logs []ddLog, err error) {
	ctx, apiClient := dogApiClient.getDDApiClient()

	var pageAttribs *datadog.LogsListRequestPage
	if *pageCursor == "" {
		pageAttribs = &datadog.LogsListRequestPage{
			Limit: datadog.PtrInt32(maxLogsPerPage),
		}
	} else {
		pageAttribs = &datadog.LogsListRequestPage{
			Limit:  datadog.PtrInt32(maxLogsPerPage),
			Cursor: pageCursor,
		}
	}

	body := datadog.LogsListRequest{
		Filter: &datadog.LogsQueryFilter{
			Query: datadog.PtrString(fmt.Sprintf("@%s.%s.%s:>0", qryEnv, qryEnvExtraJsonFields, qryFldFixedBugs)),
			//Indexes: &[]string{
			//	"main",
			//},
			From: datadog.PtrString(before.Format(time.RFC3339)), // datadog.PtrString("2020-09-17T11:48:36+01:00"),
			To:   datadog.PtrString(now.Format(time.RFC3339)),    //datadog.PtrString("2020-09-17T12:48:36+01:00"),
		},
		Sort: datadog.LOGSSORT_TIMESTAMP_ASCENDING.Ptr(),
		Page: pageAttribs,
	}
	var resp datadog.LogsListResponse
	var r *http.Response
	fetchStart := time.Now()
	resp, r, err = apiClient.LogsApi.ListLogs(ctx, *datadog.NewListLogsOptionalParameters().WithBody(body))
	if err != nil {
		logger.Error("error calling datadog api",
			zap.Error(err),
			zap.Any("http response", r),
		)
		return
	}
	fetchDuration := time.Since(fetchStart)
	logger.Debug("log page fetch time",
		zap.Duration("fetchDuration", fetchDuration),
		zap.Int("maxLogsPerPage", maxLogsPerPage),
	)

	//links := resp.GetLinks()
	//if links.GetNext() != "" {
	//	logger.Debug("has next page", zap.String("nextUrl", links.GetNext()))
	//}

	meta := resp.GetMeta()

	warnings := meta.GetWarnings()
	if warnings != nil {
		logger.Error("warnings", zap.Any("warnings", warnings))
		err = fmt.Errorf("datadog warnings: %d, see log", len(warnings))
		return
	}

	status := meta.GetStatus()
	switch status {
	case datadog.LOGSAGGREGATERESPONSESTATUS_TIMEOUT:
		logger.Debug("status", zap.Any("status", status))
		err = fmt.Errorf("timeout getting scoring page. %+v", status)
		return
	case datadog.LOGSAGGREGATERESPONSESTATUS_DONE:
		isDone = true
		return
	default:
		// more pages to read, so carry on
	}

	nextPage := meta.GetPage()
	hasAfter := nextPage.HasAfter()
	if hasAfter {
		after := nextPage.GetAfter()
		cursor = after
		//logger.Debug("has after", zap.String("after", after))
	} else {
		cursor = ""
		// meta.status never seems to say "done", so force it here, since there is no next page
		isDone = true
	}
	responseData := resp.GetData()

	logs, err = processResponseData(responseData)

	return
}

func processResponseData(responseData []datadog.Log) (logs []ddLog, err error) {
	for _, log := range responseData {
		logStruct := ddLog{
			Id: *log.Id,
		}

		attribEnv := log.Attributes.GetAttributes()[qryEnv]
		mapAttribEnv, ok := attribEnv.(map[string]interface{})
		if !ok {
			err = fmt.Errorf("unexpected attribute map type in %+v", log.Attributes.GetAttributes())
			return
		}
		extra := extraFields{}
		for key, value := range mapAttribEnv {
			switch key {
			case qryEnvBaseTime:
				var baseTime time.Time
				baseTime, err = time.Parse(time.RFC3339, value.(string))
				if err != nil {
					return
				}
				extra.envBaseTime = baseTime
			case qryEnvExtraJsonFields:
				valueMap := value.(map[string]interface{})
				var jsonMap []byte
				jsonMap, err = json.Marshal(valueMap)
				if err != nil {
					return
				}
				extra.scoringMessage = types.ScoringMessage{}
				err = json.Unmarshal(jsonMap, &extra.scoringMessage)
				if err != nil {
					// Todo fix this, saw a case where the ScoringMessage.fixed-bug-types is not a map[string]int
					// e.g.
					// {"component":"manager","delay":524.904469846,"eventSource":"github","eventType":"pullrequestopenedaction","fixed-bug-types":{"opt":{"semgrep":{"node_password":1,"node_username":1}}},"fixed-bugs":2,"job":"01FYPDNA9ZFRBX8MD9ND0DCFCG","muse_color":"blue","notes-outside-diff":0,"notes-posted":0,"pullRequestId":803,"repositoryName":"unleash-frontend","repositoryOwner":"Unleash","topic":"analysis","triggerUser":"olav"}
					if err.Error() == "json: cannot unmarshal object into Go struct field ScoringMessage.fixed-bug-types of type int" {
						// lift rug, sweep
						logger.Error("ignoring error",
							zap.Error(err),
							zap.Any("valueMap", valueMap),
						)
						// clear the error, nothing to see here, avert your eyes
						err = nil
					} else {
						logger.Error("error unmarshalling scoring message", zap.Any("valueMap", valueMap))
						return
					}
				}
			default:
				err = fmt.Errorf("unexpected extra field key: %s", key)
				return
			}
			logStruct.Fields = extra
		}

		logs = append(logs, logStruct)
	}
	return
}

type extraFields struct {
	envBaseTime    time.Time
	scoringMessage types.ScoringMessage
}

type ddLog struct {
	Id     string
	Fields extraFields
}

// ChaseTail will loop every given interval, polling dataDog for new scoring data
func ChaseTail(pollDb db.IDBPoll, scoreDb db.IScoreDB, seconds time.Duration, processScoringMessage func(scoreDb db.IScoreDB, now time.Time, msg types.ScoringMessage) (pollErr error)) (quit chan bool, errChan chan error) {
	logger = pollDb.GetLogger()
	ticker := time.NewTicker(seconds * time.Second)
	quit = make(chan bool)

	const errBufferSize = 100
	errChan = make(chan error, errBufferSize)
	var errCount int
	go func() {
		var pollErr error
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				var logs []ddLog
				logs, pollErr = pollTheDog(pollDb, now)
				if pollErr != nil {
					logger.Error("error in polling chase", zap.Error(pollErr))
					errCount++
					if errCount < errBufferSize {
						errChan <- pollErr
					}
					continue // continue allows polling to keep running when errors occur
				}
				pollErr = processLogs(scoreDb, logs, now, processScoringMessage)
				if pollErr != nil {
					logger.Error("error in process logs chase", zap.Error(pollErr))
					errCount++
					if errCount < errBufferSize {
						errChan <- pollErr
					}
					continue // continue allows polling to keep running when errors occur
				}
			case <-quit:
				ticker.Stop()
				logger.Info("poll ticker stopped", zap.Error(pollErr))
				errChan <- pollErr
				return
			}
		}
	}()
	return
}

func processLogs(scoreDb db.IScoreDB, logs []ddLog, nowPoll time.Time, processScoringMessage func(scoreDb db.IScoreDB, now time.Time, msg types.ScoringMessage) (err error)) (err error) {
	for _, log := range logs {
		msg := log.Fields.scoringMessage
		err = processScoringMessage(scoreDb, nowPoll, msg)
		if err != nil {
			return
		}
	}
	return
}
