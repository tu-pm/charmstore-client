// Copyright 2014 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package charmcmd

var (
	ClientGetArchive  = &clientGetArchive
	CSClientServerURL = &csclientServerURL
	PluginTopicText   = pluginTopicText
	ServerURL         = serverURL
	TranslateError    = translateError
	USSOTokenPath     = ussoTokenPath
)

func ResetPluginDescriptionsResults() {
	pluginDescriptionsResults = nil
}
