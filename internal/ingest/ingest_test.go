package ingest

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestIngestWithRealCharmstore(t *testing.T) {
	c := qt.New(t)
	for _, test := range ingestTests {
		test := test
		c.Run(test.testName, func(c *qt.C) {
			c.Parallel()
			srcStore := newTestCharmstore(c)
			srcStore.addEntities(c, test.src, test.srcBaseEntities)

			destStore := newTestCharmstore(c)
			destStore.addEntities(c, test.dest, test.destBaseEntities)

			stats := Ingest(IngestParams{
				Src:       srcStore.client,
				Dest:      destStore.client,
				Whitelist: test.whitelist,
				Log:       testLogFunc(c),
			})
			c.Check(stats, qt.DeepEquals, test.expectStats)
			destStore.assertContents(c, test.expectContents, test.expectBaseEntityContents)

			// Try again; we should transfer nothing and the contents should
			// remain the same.
			stats = Ingest(IngestParams{
				Src:       srcStore.client,
				Dest:      destStore.client,
				Whitelist: test.whitelist,
			})
			expectStats := test.expectStats
			expectStats.ArchivesCopiedCount = 0
			c.Check(stats, qt.DeepEquals, expectStats)
			destStore.assertContents(c, test.expectContents, test.expectBaseEntityContents)
		})
	}
}
