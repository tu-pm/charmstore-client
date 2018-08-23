// Copyright 2018 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/juju/charmstore-client/internal/ingest"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charmrepo.v4/csclient/params"
)

// parseError is the error returned when a whitelist cannot be parsed.
type parseError struct {
	Filename string
	Line     int
	Err      error
}

func (e *parseError) Error() string {
	return fmt.Sprintf("%s:%d: %v", e.Filename, e.Line, e.Err)
}

// parseWhitelist parses a whitelist of charms and bundles read from r
// and returns a slice containing all of them.
//
// One entity is defined per line, with the entity id first, followed
// by all the channels that should be considered for the entity,
// separated by white space.
//
// If the entity id has no specified revision, the most recent published
// revision for the channels will be specified.
func parseWhitelist(filename string, r io.Reader) ([]ingest.WhitelistEntity, error) {
	var entities []ingest.WhitelistEntity
	fileScanner := bufio.NewScanner(r)
	lineNum := 0

	for fileScanner.Scan() {
		lineNum++

		entity, err := parseLine(fileScanner.Text())
		if err != nil {
			return nil, &parseError{
				Filename: filename,
				Line:     lineNum,
				Err:      err,
			}
		}

		if entity.EntityId == "" {
			// Empty line, ignore
			continue
		}
		entities = append(entities, entity)
	}

	if err := fileScanner.Err(); err != nil {
		return nil, errgo.Mask(err)
	}

	return entities, nil
}

// parseLine parses the given line into a WhitelistEntity. If the line is empty, it
// returns the zero WhitelistEntity value.
func parseLine(line string) (ingest.WhitelistEntity, error) {
	var entity ingest.WhitelistEntity

	fields := strings.Fields(line)
	if len(fields) == 0 {
		// Empty line
		return entity, nil
	}

	entity.EntityId = fields[0]
	entity.Channels = make([]params.Channel, len(fields)-1)
	for i, channelStr := range fields[1:] {
		channel := params.Channel(channelStr)
		if !params.ValidChannels[channel] {
			return entity, fmt.Errorf("invalid channel %q for entity %q", channelStr, entity.EntityId)
		}
		entity.Channels[i] = channel
	}

	return entity, nil
}
