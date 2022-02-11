//
// Copyright 2021-present Sonatype Inc.
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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"net/http"
	"os"
	"time"
)

type webflowConfig struct {
	// variable baseAPI allows for changes to url for testing
	baseAPI               string // typically default to: WebflowApiBase
	token                 string
	campaignCollection    string // campaign CMS collection id
	participantCollection string // participant CMS collection id
}

const (
	WebflowApiBase string = "https://api.webflow.com"
)

var upstreamConfig = webflowConfig{
	baseAPI: WebflowApiBase,
}

func setupUpstream() {
	upstreamConfig.token = os.Getenv("WEBFLOW_TOKEN")
	upstreamConfig.campaignCollection = os.Getenv("WEBFLOW_CAMPAIGN_COLLECTION_ID")
	upstreamConfig.participantCollection = os.Getenv("WEBFLOW_COLLECTION_ID")
}

type leaderboardItem struct {
	UserName           string `json:"name"`
	Slug               string `json:"slug"`
	Score              int    `json:"score"`
	CampaignUpstreamId string `json:"campaign-reference"`
	Archived           bool   `json:"_archived"`
	Draft              bool   `json:"_draft"`
}

type leaderboardPayload struct {
	Fields leaderboardItem `json:"fields"`
}

type leaderboardResponse struct {
	Id string `json:"_id"`
}

const msgPatternCreateErrorCampaign = "could not create upstream campaign. response status: %s"
const msgPatternActivateErrorCampaign = "could not activate upstream campaign. response status: %s"
const msgPatternCreateErrorParticipant = "could not create upstream participant. response status: %s"
const msgPatternDeleteErrorParticipant = "could not delete upstream participant. response status: %s"

func doUpstreamRequest(c echo.Context, req *http.Request, errMsgPattern string) (res *http.Response, err error) {
	requestHeaderSetup(req)

	res, err = getNetClient().Do(req)
	if err != nil {
		return
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		c.Logger().Debug(res)
		var responseBody []byte
		_, _ = res.Body.Read(responseBody)
		c.Logger().Debug(responseBody)
		// return a real error to the caller to indicate failure
		err = &CreateError{MsgPattern: errMsgPattern, Status: res.Status}
		if errContext := c.String(http.StatusInternalServerError, err.Error()); errContext != nil {
			err = errContext
		}
		return
	}
	return
}

func upstreamNewCampaign(c echo.Context, newCampaign *campaignStruct, isActive bool) (id string, err error) {
	item := leaderboardCampaign{
		CampaignName: newCampaign.Name,
		Slug:         newCampaign.Name,
		CreateOrder:  newCampaign.CreatedOrder,
		Active:       isActive,
		Note:         "",
		Archived:     false,
		Draft:        false,
	}

	payload := leaderboardCampaignPayload{
		Fields: item,
	}

	var body []byte
	body, err = json.Marshal(payload)
	if err != nil {
		return
	}

	var req *http.Request
	req, err = http.NewRequest("POST", fmt.Sprintf("%s/collections/%s/items?live=true", upstreamConfig.baseAPI, upstreamConfig.campaignCollection), bytes.NewReader(body))
	if err != nil {
		return
	}

	var res *http.Response
	res, err = doUpstreamRequest(c, req, msgPatternCreateErrorCampaign)
	if err != nil {
		return
	}

	var response leaderboardCampaignResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return
	}
	id = response.Id

	c.Logger().Debugf("created new upstream campaign: leaderboardCampaign: %+v", item)
	return
}

const sqlSelectUpstreamIdCampaign = `SELECT upstream_id FROM campaign WHERE name = $1`

// lookup the Upstream_id for the campaign name
func getCampaignUpstreamId(c echo.Context, campaignName string) (campaignUpstreamId string, err error) {
	err = db.QueryRow(sqlSelectUpstreamIdCampaign, campaignName).
		Scan(&campaignUpstreamId)
	if err != nil {
		c.Logger().Errorf("error reading campaign upstream id. campaignName: %s, err: %+v", campaignName, err)
		return
	}

	return
}

func isCampaignActive(campaign campaignStruct, now time.Time) (isActive bool, err error) {
	isActive = now.After(campaign.StartOn) && (now.Before(campaign.EndOn))
	return
}

func upstreamActivateCampaign(c echo.Context, campaign campaignStruct, isActive bool) (id string, err error) {
	item := leaderboardCampaign{
		CampaignName: campaign.Name,
		Slug:         campaign.Name,
		CreateOrder:  campaign.CreatedOrder,
		Active:       isActive,
	}

	payload := leaderboardCampaignPayload{
		Fields: item,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("PUT", fmt.Sprintf("%s/collections/%s/items/%s?live=true",
		upstreamConfig.baseAPI, upstreamConfig.campaignCollection, campaign.UpstreamId), bytes.NewReader(body))
	if err != nil {
		return
	}

	var res *http.Response
	res, err = doUpstreamRequest(c, req, msgPatternActivateErrorCampaign)
	if err != nil {
		return
	}

	var response leaderboardCampaignResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return
	}
	id = response.Id

	c.Logger().Debugf("updated upstream campaign: leaderboardCampaign: %+v", item)
	return
}

func updateUpstreamCampaignActiveStatus(c echo.Context, campaignName string) (err error) {
	// update upstream active status
	campaignFromDB, err := getCampaign(campaignName)
	if err != nil {
		return
	}
	now := time.Now()
	isActive, err := isCampaignActive(campaignFromDB, now)
	if err != nil {
		return
	}
	_, err = upstreamActivateCampaign(c, campaignFromDB, isActive)
	if err != nil {
		return
	}

	return
}

func upstreamNewParticipant(c echo.Context, p participant) (id string, err error) {
	// @todo Sanity check the campaign/scp/login doesn't already exist before creating Upstream record

	campaignUpstreamId, err := getCampaignUpstreamId(c, p.CampaignName)
	if err != nil {
		c.Logger().Errorf("error reading campaign upstream id for new participant: %+v", p)
		return
	}
	item := leaderboardItem{}
	item.CampaignUpstreamId = campaignUpstreamId
	item.UserName = p.LoginName
	item.Slug = p.LoginName
	item.Score = 0
	item.Archived = false
	item.Draft = false

	payload := leaderboardPayload{}
	payload.Fields = item

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/collections/%s/items?live=true", upstreamConfig.baseAPI, upstreamConfig.participantCollection), bytes.NewReader(body))
	if err != nil {
		return
	}

	var res *http.Response
	res, err = doUpstreamRequest(c, req, msgPatternCreateErrorParticipant)
	if err != nil {
		return
	}

	var response leaderboardResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return
	}
	id = response.Id

	c.Logger().Debugf("created new upstream user: leaderboardItem: %+v", item)
	return
}

// see: https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
func getNetClient() (netClient *http.Client) {
	netClient = &http.Client{
		Timeout: time.Second * 10,
	}
	return
}

func requestHeaderSetup(req *http.Request) {
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", upstreamConfig.token))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("accept-version", "1.0.0")
}

type ParticipantUpdateError struct {
	Status string
}

func (e *ParticipantUpdateError) Error() string {
	return fmt.Sprintf("could not update score. response status: %s", e.Status)
}

func createNewWebflowId(c echo.Context, campaignFromRequest *campaignStruct) (webflowId string, err error) {
	now := time.Now()
	isActive := now.After(campaignFromRequest.StartOn) && (now.Before(campaignFromRequest.EndOn))
	webflowId, err = upstreamNewCampaign(c, campaignFromRequest, isActive)
	return
}

// delete from upstream - warning: slugs are cached until webflow republishes site. create, delete, create will complain
func upstreamDeleteParticipant(c echo.Context, participantUpstreamId string) (id string, err error) {
	req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/collections/%s/items/%s?live=true",
		upstreamConfig.baseAPI, upstreamConfig.participantCollection, participantUpstreamId), nil)
	if err != nil {
		return
	}

	var res *http.Response
	res, err = doUpstreamRequest(c, req, msgPatternDeleteErrorParticipant)
	if err != nil {
		return
	}

	var response leaderboardResponse
	err = json.NewDecoder(res.Body).Decode(&response)
	if err != nil {
		return
	}
	id = response.Id

	c.Logger().Debugf("deleted upstream user: participantUpstreamId: %s", participantUpstreamId)
	return
}

func upstreamUpdateScore(c echo.Context, webflowId string, score int) (err error) {

	var payload struct {
		Fields struct {
			Score int `json:"score"`
		} `json:"fields"`
	}
	payload.Fields.Score = score

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	url := fmt.Sprintf("%s/collections/%s/items/%s?live=true", upstreamConfig.baseAPI, upstreamConfig.participantCollection, webflowId)
	req, err := http.NewRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		return
	}
	requestHeaderSetup(req)

	res, err := getNetClient().Do(req)
	if err != nil {
		return
	} else if res.StatusCode < 200 || res.StatusCode >= 300 {
		c.Logger().Debug(req)
		var responseBody []byte
		_, _ = res.Body.Read(responseBody)
		c.Logger().Debug(responseBody)
		// return a real error to the caller to indicate failure
		err = &ParticipantUpdateError{res.Status}
		if errContext := c.String(http.StatusInternalServerError, err.Error()); errContext != nil {
			err = errContext
		}
	}

	return
}
