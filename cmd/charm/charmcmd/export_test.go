// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

import "gopkg.in/juju/charm.v6-unstable"

var (
	ClientGetArchive            = &clientGetArchive
	CSClientServerURL           = &csclientServerURL
	GetExtraInfo                = getExtraInfo
	MapLogEntriesToVcsRevisions = mapLogEntriesToVcsRevisions
	ParseGitLog                 = parseGitLog
	PluginTopicText             = pluginTopicText
	ServerURL                   = serverURL
	UploadResource              = &uploadResource
	PublishCharm                = &publishCharm
)

func NewListResourcesCommand(
	newCharmstoreClient NewCharmstoreClientFn,
	formatTabular func(interface{}) ([]byte, error),
	username,
	password string,
	charmID *charm.URL,
) *listResourcesCommand {
	return &listResourcesCommand{
		NewCharmstoreClient: newCharmstoreClient,
		formatTabular:       formatTabular,
		username:            username,
		password:            password,
		charmID:             charmID,
	}
}

var FormatTabular = formatTabular
