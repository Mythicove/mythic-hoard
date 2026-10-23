// Package search 提供搜索实现（Meilisearch，高档）。
package search

import (
	"context"
	"encoding/json"

	"github.com/Mythicove/shared/search"
	"github.com/meilisearch/meilisearch-go"
)

// Meili 是 Meilisearch 实现。
type Meili struct {
	client meilisearch.ServiceManager
}

var _ search.Search = (*Meili)(nil)

// NewMeili 构造 Meilisearch 客户端。
func NewMeili(host, apiKey string) *Meili {
	return &Meili{client: meilisearch.New(host, meilisearch.WithAPIKey(apiKey))}
}

func (m *Meili) Index(_ context.Context, index string, docs []search.Document) error {
	msDocs := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		doc := map[string]any{"id": d.ID}
		for k, v := range d.Fields {
			doc[k] = v
		}
		for k, v := range d.Facets {
			doc[k] = v
		}
		msDocs = append(msDocs, doc)
	}
	if len(msDocs) == 0 {
		return nil
	}
	_, err := m.client.Index(index).AddDocuments(msDocs, nil)
	return err
}

func (m *Meili) Delete(_ context.Context, index, id string) error {
	_, err := m.client.Index(index).DeleteDocument(id, nil)
	return err
}

func (m *Meili) Query(_ context.Context, index, q string, facets map[string][]string, limit, offset int) ([]string, error) {
	filters := make([]string, 0, len(facets))
	for k, vals := range facets {
		for _, v := range vals {
			filters = append(filters, k+" = "+v)
		}
	}
	req := &meilisearch.SearchRequest{
		Limit:  int64(limit),
		Offset: int64(offset),
		Filter: filters,
	}
	res, err := m.client.Index(index).Search(q, req)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res.Hits))
	for _, h := range res.Hits {
		raw, ok := h["id"]
		if !ok {
			continue
		}
		var id string
		if json.Unmarshal(raw, &id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
