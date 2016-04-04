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
	FormatTabular               = formatTabular
	TabularFormatter            = tabularFormatter
	TranslateError              = translateError
	USSOTokenPath               = ussoTokenPath
)

func NewListResourcesCommand(
	newCharmstoreClient NewCharmstoreClientFn,
	formatTabular func(interface{}) ([]byte, error),
	username,
	password string,
	charmID *charm.URL,
) *listResourcesCommand {
	cmd := &listResourcesCommand{
		newCharmstoreClient: newCharmstoreClient,
		formatTabular:       formatTabular,
		username:            username,
		password:            password,
		charmID:             charmID,
	}
	if cmd.formatTabular == nil {
		cmd.formatTabular = tabularFormatter
	}
	return cmd
}

func (c *listResourcesCommand) CharmID() *charm.URL {
	return c.charmID
}

func (c *listResourcesCommand) Username() string {
	return c.username
}

func (c *listResourcesCommand) Password() string {
	return c.password
}

func (c *listResourcesCommand) Channel() string {
	return c.channel
}

func ResetPluginDescriptionsResults() {
	pluginDescriptionsResults = nil
}
