package ingest

import (
	"testing"

	qt "github.com/frankban/quicktest"
)

func TestIngestWithRealCharmstore(t *testing.T) {
	c := qt.New(t)
	for _, test := range ingestTests {
		c.Run(test.testName, func(c *qt.C) {
			c.Parallel()
			srcStore := newTestCharmstore(c)
			srcStore.addEntities(c, test.src)

			destStore := newTestCharmstore(c)
			destStore.addEntities(c, test.dest)

			stats := ingest(ingestParams{
				src:       charmstoreShim{srcStore.client},
				dest:      charmstoreShim{destStore.client},
				whitelist: test.whitelist,
			})
			c.Check(stats, qt.DeepEquals, test.expectStats)
			destStore.assertContents(c, test.expectContents)

			// Try again; we should transfer nothing and the contents should
			// remain the same.
			stats = ingest(ingestParams{
				src:       charmstoreShim{srcStore.client},
				dest:      charmstoreShim{destStore.client},
				whitelist: test.whitelist,
			})
			expectStats := test.expectStats
			expectStats.ArchivesCopiedCount = 0
			c.Check(stats, qt.DeepEquals, expectStats)
			destStore.assertContents(c, test.expectContents)
		})
	}
}
