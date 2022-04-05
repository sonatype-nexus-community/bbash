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

package db

import (
	"database/sql"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/sonatype-nexus-community/bbash/internal/types"
	"go.uber.org/zap"
	"time"
)

type IScoreDB interface {
	SelectPriorScore(participantToScore *types.ParticipantStruct, msg *types.ScoringMessage) (oldPoints float64)
	InsertScoringEvent(participantToScore *types.ParticipantStruct, msg *types.ScoringMessage, newPoints float64) (err error)
	UpdateParticipantScore(participant *types.ParticipantStruct, delta float64) (err error)
}

type IBBashDB interface {
	MigrateDB(migrateSourceURL string) error

	GetSourceControlProviders() (scps []types.SourceControlProviderStruct, err error)

	InsertCampaign(campaign *types.CampaignStruct) (guid string, err error)
	UpdateCampaign(campaign *types.CampaignStruct) (guid string, err error)
	GetCampaign(campaignName string) (campaign *types.CampaignStruct, err error)
	GetCampaigns() (campaigns []types.CampaignStruct, err error)
	GetActiveCampaigns(now time.Time) (activeCampaigns []types.CampaignStruct, err error)

	InsertOrganization(organization *types.OrganizationStruct) (guid string, err error)
	GetOrganizations() (organizations []types.OrganizationStruct, err error)
	DeleteOrganization(scpName, orgName string) (rowsAffected int64, err error)
	ValidOrganization(msg *types.ScoringMessage) (orgExists bool, err error)

	SelectParticipantsToScore(msg *types.ScoringMessage, now time.Time) (participantsToScore []types.ParticipantStruct, err error)
	SelectPointValue(msg *types.ScoringMessage, campaignName, bugType string) (pointValue float64)
	IScoreDB

	InsertParticipant(participant *types.ParticipantStruct) (err error)
	SelectParticipantDetail(campaignName, scpName, loginName string) (participant *types.ParticipantStruct, err error)
	SelectParticipantsInCampaign(campaignName string) (participants []types.ParticipantStruct, err error)
	UpdateParticipant(participant *types.ParticipantStruct) (rowsAffected int64, err error)
	DeleteParticipant(campaign, scpName, loginName string) (participantId string, err error)
	UpdateParticipantTeam(teamName, campaignName, scpName, loginName string) (rowsAffected int64, err error)

	InsertTeam(team *types.TeamStruct) (err error)

	InsertBug(bug *types.BugStruct) (err error)
	UpdateBug(bug *types.BugStruct) (rowsAffected int64, err error)
	SelectBugs() (bugs []types.BugStruct, err error)
}

type BBashDB struct {
	db     *sql.DB
	logger *zap.Logger
}

// Roll that beautiful bean footage
var _ IBBashDB = (*BBashDB)(nil)

func New(db *sql.DB, logger *zap.Logger) *BBashDB {
	return &BBashDB{db: db, logger: logger}
}

func (p *BBashDB) MigrateDB(migrateSourceURL string) (err error) {

	driver, err := postgres.WithInstance(p.db, &postgres.Config{})
	if err != nil {
		return
	}

	m, err := migrate.NewWithDatabaseInstance(
		migrateSourceURL,
		"postgres", driver)
	if err != nil {
		return
	}

	if err = m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			// we can ignore (and clear) the "no change" error
			err = nil
		}
	}
	return
}

const sqlSelectSourceControlProvider = `SELECT * FROM source_control_provider`

func (p *BBashDB) GetSourceControlProviders() (scps []types.SourceControlProviderStruct, err error) {
	var rows *sql.Rows
	rows, err = p.db.Query(sqlSelectSourceControlProvider)
	if err != nil {
		return
	}

	for rows.Next() {
		scp := types.SourceControlProviderStruct{}
		err = rows.Scan(&scp.ID, &scp.SCPName, &scp.Url)
		if err != nil {
			return
		}
		scps = append(scps, scp)
	}
	return
}

const sqlInsertCampaign = `INSERT INTO campaign 
		(name, start_on, end_on) 
		VALUES ($1, $2, $3)
		RETURNING Id`

func (p *BBashDB) InsertCampaign(campaign *types.CampaignStruct) (guid string, err error) {
	err = p.db.QueryRow(
		sqlInsertCampaign,
		campaign.Name,
		campaign.StartOn,
		campaign.EndOn,
	).Scan(&guid)
	return
}

const sqlUpdateCampaign = `UPDATE campaign
		SET start_on = $1,
			end_on = $2		
		WHERE name = $3
		RETURNING id`

func (p *BBashDB) UpdateCampaign(campaign *types.CampaignStruct) (guid string, err error) {
	err = p.db.QueryRow(
		sqlUpdateCampaign,
		campaign.StartOn,
		campaign.EndOn,
		campaign.Name,
	).Scan(&guid)
	return
}

const sqlSelectCampaign = `SELECT ID, name, created_on, create_order, start_on, end_on, note 
	FROM campaign
	WHERE name = $1`

func (p *BBashDB) GetCampaign(campaignName string) (campaign *types.CampaignStruct, err error) {
	rows, err := p.db.Query(sqlSelectCampaign, campaignName)
	if err != nil {
		return
	}

	campaign = &types.CampaignStruct{}
	for rows.Next() {
		err = rows.Scan(&campaign.ID, &campaign.Name, &campaign.CreatedOn, &campaign.CreatedOrder, &campaign.StartOn, &campaign.EndOn, &campaign.Note)
		if err != nil {
			return
		}
	}
	return
}

const sqlSelectCampaigns = `SELECT ID, name, created_on, create_order, start_on, end_on, note FROM campaign`

func (p *BBashDB) GetCampaigns() (campaigns []types.CampaignStruct, err error) {
	rows, err := p.db.Query(
		sqlSelectCampaigns)
	if err != nil {
		return
	}

	for rows.Next() {
		campaign := types.CampaignStruct{}
		err = rows.Scan(&campaign.ID, &campaign.Name, &campaign.CreatedOn, &campaign.CreatedOrder, &campaign.StartOn, &campaign.EndOn, &campaign.Note)
		if err != nil {
			return
		}
		campaigns = append(campaigns, campaign)
	}
	return
}

const sqlSelectCurrentCampaigns = `SELECT * FROM campaign
		WHERE $1 >= start_on
			AND $1 < end_on
		ORDER BY start_on`

func (p *BBashDB) GetActiveCampaigns(now time.Time) (activeCampaigns []types.CampaignStruct, err error) {
	rows, err := p.db.Query(sqlSelectCurrentCampaigns, now)
	if err != nil {
		return
	}

	for rows.Next() {
		activeCampaign := types.CampaignStruct{}

		err = rows.Scan(&activeCampaign.ID, &activeCampaign.Name, &activeCampaign.CreatedOn, &activeCampaign.CreatedOrder, &activeCampaign.StartOn, &activeCampaign.EndOn, &activeCampaign.Note)
		if err != nil {
			return
		}
		activeCampaigns = append(activeCampaigns, activeCampaign)
	}

	return
}

const sqlInsertOrganization = `INSERT INTO organization
		(fk_scp, organization)
		VALUES ((SELECT id FROM source_control_provider WHERE name = $1), $2)
		RETURNING Id`

func (p *BBashDB) InsertOrganization(organization *types.OrganizationStruct) (guid string, err error) {
	err = p.db.QueryRow(sqlInsertOrganization, organization.SCPName, organization.Organization).
		Scan(&guid)
	return
}

const sqlSelectOrganizations = `SELECT
		organization.Id,
        Name,
        Organization
		FROM organization
		INNER JOIN source_control_provider ON fk_scp = source_control_provider.Id`

func (p *BBashDB) GetOrganizations() (organizations []types.OrganizationStruct, err error) {
	rows, err := p.db.Query(sqlSelectOrganizations)
	if err != nil {
		return
	}

	for rows.Next() {
		org := types.OrganizationStruct{}
		err = rows.Scan(&org.ID, &org.SCPName, &org.Organization)
		if err != nil {
			return
		}
		organizations = append(organizations, org)
	}
	return
}

const sqlDeleteOrganization = `DELETE FROM organization
	WHERE fk_scp = (SELECT id from source_control_provider WHERE name = $1) 
	AND Organization = $2`

func (p *BBashDB) DeleteOrganization(scpName, orgName string) (rowsAffected int64, err error) {
	res, err := p.db.Exec(sqlDeleteOrganization, scpName, orgName)
	if err != nil {
		return
	}
	rowsAffected, _ = res.RowsAffected()
	return
}

const sqlSelectOrganizationExists = `SELECT EXISTS(
		SELECT Id FROM organization
		WHERE fk_scp = (SELECT id from source_control_provider WHERE LOWER(name) = $1) AND Organization = $2)`

func (p *BBashDB) ValidOrganization(msg *types.ScoringMessage) (orgExists bool, err error) {
	row := p.db.QueryRow(sqlSelectOrganizationExists, msg.EventSource, msg.RepoOwner)
	err = row.Scan(&orgExists)
	if err != nil {
		p.logger.Error("organization read error", zap.Any("msg", msg), zap.Error(err))
		return
	}
	return
}

const sqlSelectParticipantId = `SELECT
		participant.Id,
        campaign.name,
       	source_control_provider.name,
        participant.login_name,
        team.name
		FROM participant
		INNER JOIN campaign ON campaign.Id = fk_campaign
		INNER JOIN source_control_provider ON source_control_provider.Id = fk_scp
		LEFT JOIN team ON team.Id = participant.fk_team
		WHERE $1 >= campaign.start_on
			AND $1 < campaign.end_on
		    AND LOWER(source_control_provider.name) = $2 
			AND login_name = $3`

func (p *BBashDB) SelectParticipantsToScore(msg *types.ScoringMessage, now time.Time) (participantsToScore []types.ParticipantStruct, err error) {
	// Check if participant is registered for an active campaign
	var rows *sql.Rows
	rows, err = p.db.Query(sqlSelectParticipantId, now, msg.EventSource, msg.TriggerUser)
	if err != nil {
		p.logger.Error("skip score-error reading participant", zap.Any("msg", msg), zap.Error(err))
		return
	}
	for rows.Next() {
		partier := types.ParticipantStruct{}
		var nullableTeamName sql.NullString
		// note: reads the db (capitalized) scpName
		err = rows.Scan(&partier.ID, &partier.CampaignName, &partier.ScpName, &partier.LoginName, &nullableTeamName)
		if nullableTeamName.Valid {
			partier.TeamName = nullableTeamName.String
		}
		if err != nil {
			p.logger.Error("skip score-error scanning participant", zap.Any("msg", msg), zap.Error(err))
			return
		}
		participantsToScore = append(participantsToScore, partier)
	}
	return
}

const sqlSelectPointValue = `SELECT pointValue FROM bug 
	INNER JOIN campaign ON campaign.Id = fk_campaign	
	WHERE fk_campaign = (SELECT campaign.Id FROM campaign WHERE name = $1) 
	  AND category = $2`

func (p *BBashDB) SelectPointValue(msg *types.ScoringMessage, campaignName, bugType string) (pointValue float64) {
	row := p.db.QueryRow(sqlSelectPointValue, campaignName, bugType)
	pointValue = 1
	if err := row.Scan(&pointValue); err != nil {
		// ignore error from scan operation
		p.logger.Debug("ignoring missing pointValue",
			zap.String("bugType", bugType), zap.Error(err), zap.Any("msg", msg))
	}
	return
}

const sqlUpdateParticipantScore = `UPDATE participant 
		SET Score = Score + $1 
		WHERE id = $2 
		RETURNING Score`

func (p *BBashDB) UpdateParticipantScore(participant *types.ParticipantStruct, delta float64) (err error) {
	var score int
	row := p.db.QueryRow(sqlUpdateParticipantScore, delta, participant.ID)
	err = row.Scan(&score)
	return
}

const sqlScoreQuery = `SELECT points
			FROM scoring_event
			WHERE fk_campaign = (SELECT id FROM campaign WHERE name = $1)
			    AND fk_scp = (SELECT id FROM source_control_provider WHERE name = $2)
			    AND repoOwner = $3
				AND repoName = $4
				AND pr = $5`

func (p *BBashDB) SelectPriorScore(participantToScore *types.ParticipantStruct, msg *types.ScoringMessage) (oldPoints float64) {
	row := p.db.QueryRow(sqlScoreQuery, participantToScore.CampaignName, participantToScore.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest)
	oldPoints = 0
	err := row.Scan(&oldPoints)
	if err != nil {
		// ignore error case from scan when no row exists, will occur when this is a new score event
		p.logger.Debug("ignoring likely new score event", zap.Error(err), zap.Any("ScoringMessage", msg))
	}
	return
}

const sqlInsertScoringEvent = `INSERT INTO scoring_event
			(fk_campaign, fk_scp, repoOwner, repoName, pr, username, points)
			VALUES ((SELECT id FROM campaign WHERE name = $1), 
			        (SELECT id FROM source_control_provider WHERE name = $2),
			        $3, $4, $5, $6, $7)
			ON CONFLICT (fk_campaign, fk_scp, repoOwner, repoName, pr) DO
				UPDATE SET points = $7`

func (p *BBashDB) InsertScoringEvent(participantToScore *types.ParticipantStruct, msg *types.ScoringMessage, newPoints float64) (err error) {
	_, err = p.db.Exec(sqlInsertScoringEvent, participantToScore.CampaignName, participantToScore.ScpName, msg.RepoOwner, msg.RepoName, msg.PullRequest, msg.TriggerUser, newPoints)
	return
}

const sqlInsertParticipant = `INSERT INTO participant 
		(fk_scp, fk_campaign, login_name, Email, DisplayName, Score) 
		VALUES ((SELECT Id FROM source_control_provider WHERE Name = $1),
		        (SELECT Id FROM campaign WHERE name = $2),
		        $3, $4, $5, $6)
		RETURNING Id, Score, JoinedAt`

func (p *BBashDB) InsertParticipant(participant *types.ParticipantStruct) (err error) {
	err = p.db.QueryRow(
		sqlInsertParticipant,
		participant.ScpName,
		participant.CampaignName,
		participant.LoginName,
		participant.Email,
		participant.DisplayName,
		0,
	).Scan(&participant.ID, &participant.Score, &participant.JoinedAt)
	if err != nil {
		p.logger.Error("error inserting participant", zap.Any("participant", participant), zap.Error(err))
	}
	return
}

const sqlInsertTeam = `INSERT INTO team
		(fk_campaign, name)
		VALUES ((SELECT id FROM campaign WHERE name = $1), $2)
		RETURNING Id`

func (p *BBashDB) InsertTeam(team *types.TeamStruct) (err error) {
	err = p.db.QueryRow(
		sqlInsertTeam,
		team.CampaignName,
		team.Name).Scan(&team.Id)
	return
}

const sqlSelectParticipantDetail = `SELECT 
		participant.Id, campaign.name, source_control_provider.name, login_name, Email, DisplayName, Score, team.name, JoinedAt
		FROM participant
		LEFT JOIN team ON team.Id = participant.fk_team
		INNER JOIN campaign ON campaign.Id = participant.fk_campaign
		INNER JOIN source_control_provider ON participant.fk_scp = source_control_provider.Id
		WHERE campaign.name = $1
		  AND source_control_provider.name = $2 
		  AND participant.login_name = $3`

func (p *BBashDB) SelectParticipantDetail(campaignName, scpName, loginName string) (participant *types.ParticipantStruct, err error) {
	row := p.db.QueryRow(sqlSelectParticipantDetail, campaignName, scpName, loginName)

	participant = new(types.ParticipantStruct)
	var nullableTeamName sql.NullString
	err = row.Scan(&participant.ID,
		&participant.CampaignName,
		&participant.ScpName,
		&participant.LoginName,
		&participant.Email,
		&participant.DisplayName,
		&participant.Score,
		&nullableTeamName,
		&participant.JoinedAt,
	)
	if err != nil {
		p.logger.Error("getParticipantDetail scan error", zap.Error(err))
		return
	}
	if nullableTeamName.Valid {
		participant.TeamName = nullableTeamName.String
	}
	return
}

const sqlSelectParticipantsByCampaign = `SELECT
		participant.Id, campaign.name, source_control_provider.name, login_name, Email, DisplayName, Score, team.name, JoinedAt 
		FROM participant
		LEFT JOIN team ON participant.fk_team = team.Id
		INNER JOIN campaign ON participant.fk_campaign = campaign.Id
		INNER JOIN source_control_provider ON participant.fk_scp = source_control_provider.Id
		WHERE campaign.name = $1`

func (p *BBashDB) SelectParticipantsInCampaign(campaignName string) (participants []types.ParticipantStruct, err error) {
	rows, err := p.db.Query(sqlSelectParticipantsByCampaign, campaignName)
	if err != nil {
		return
	}

	for rows.Next() {
		participant := new(types.ParticipantStruct)
		var nullableTeamName sql.NullString
		err = rows.Scan(
			&participant.ID,
			&participant.CampaignName,
			&participant.ScpName,
			&participant.LoginName,
			&participant.Email,
			&participant.DisplayName,
			&participant.Score,
			&nullableTeamName,
			&participant.JoinedAt,
		)
		if err != nil {
			return
		}
		if nullableTeamName.Valid {
			participant.TeamName = nullableTeamName.String
		}
		participants = append(participants, *participant)
	}
	return
}

const sqlUpdateParticipant = `UPDATE participant 
		SET 
		    fk_campaign = (SELECT Id FROM campaign WHERE name = $1),
		    fk_scp = (SELECT Id FROM source_control_provider WHERE name = $2),
		    login_name = $3,
		    Email = $4,
		    DisplayName = $5,
		    Score = $6,
		    fk_team = (SELECT Id FROM team WHERE name = $7)		    
		WHERE Id = $8`

func (p *BBashDB) UpdateParticipant(participant *types.ParticipantStruct) (rowsAffected int64, err error) {
	res, err := p.db.Exec(
		sqlUpdateParticipant,
		participant.CampaignName,
		participant.ScpName,
		participant.LoginName,
		participant.Email,
		participant.DisplayName,
		participant.Score,
		participant.TeamName,
		participant.ID,
	)
	if err != nil {
		return
	}

	rowsAffected, err = res.RowsAffected()
	return
}

const sqlDeleteParticipant = `DELETE FROM participant WHERE
                          fk_campaign = (SELECT id from campaign where name =$1)
                          AND fk_scp = (SELECT id from source_control_provider where name =$2)
                          AND login_name = $3
                          RETURNING id`

func (p *BBashDB) DeleteParticipant(campaign, scpName, loginName string) (participantId string, err error) {
	err = p.db.QueryRow(sqlDeleteParticipant, campaign, scpName, loginName).Scan(&participantId)
	if err != nil {
		p.logger.Error("error deleting participant",
			zap.String("campaign", campaign), zap.String("scpName", scpName),
			zap.String("loginName", loginName), zap.Error(err))
	}
	return
}

const sqlUpdateParticipantTeam = `UPDATE participant 
		SET fk_team = (SELECT Id FROM team WHERE name = $1)
		WHERE fk_campaign = (SELECT id FROM campaign WHERE name = $2)
		 AND fk_scp = (SELECT id FROM source_control_provider WHERE name = $3)
		 AND login_name = $4`

func (p *BBashDB) UpdateParticipantTeam(teamName, campaignName, scpName, loginName string) (rowsAffected int64, err error) {
	res, err := p.db.Exec(
		sqlUpdateParticipantTeam,
		teamName,
		campaignName,
		scpName,
		loginName)
	if err != nil {
		return
	}
	rowsAffected, err = res.RowsAffected()
	if err != nil {
		return
	}
	return
}

const sqlInsertBug = `INSERT INTO bug
		(fk_campaign, category, pointValue)
		VALUES ((SELECT id FROM campaign WHERE name = $1), $2, $3)
		RETURNING ID`

func (p *BBashDB) InsertBug(bug *types.BugStruct) (err error) {
	err = p.db.QueryRow(sqlInsertBug, bug.Campaign, bug.Category, bug.PointValue).Scan(&bug.Id)
	if err != nil {
		p.logger.Error("error inserting bug", zap.Any("bug", bug), zap.Error(err))
		return
	}
	return
}

const sqlUpdateBug = `UPDATE bug
		SET pointValue = $1
		WHERE fk_campaign = (SELECT id FROM campaign WHERE name = $2) AND category = $3`

func (p *BBashDB) UpdateBug(bug *types.BugStruct) (rowsAffected int64, err error) {
	res, err := p.db.Exec(sqlUpdateBug, bug.PointValue, bug.Campaign, bug.Category)
	if err != nil {
		return
	}
	rowsAffected, err = res.RowsAffected()
	return
}

const sqlSelectBugs = `SELECT bug.id, campaign.name, category, pointValue FROM bug
		INNER JOIN campaign ON fk_campaign = campaign.Id`

func (p *BBashDB) SelectBugs() (bugs []types.BugStruct, err error) {
	rows, err := p.db.Query(sqlSelectBugs)
	if err != nil {
		return
	}

	for rows.Next() {
		bug := types.BugStruct{}
		err = rows.Scan(&bug.Id, &bug.Campaign, &bug.Category, &bug.PointValue)
		if err != nil {
			return
		}
		bugs = append(bugs, bug)
	}
	return
}
