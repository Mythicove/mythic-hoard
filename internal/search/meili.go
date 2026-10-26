// Package search 提供搜索实现（Meilisearch，高档）。
package search

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/Mythicove/shared/search"
	"github.com/meilisearch/meilisearch-go"
)

// Meili 是 Meilisearch 实现。
type Meili struct {
	client meilisearch.ServiceManager

	mu          sync.Mutex
	provisioned map[string]string // index uid -> 已应用的 facet 键集合签名
}

var _ search.Search = (*Meili)(nil)

// maxTotalHits 抬高 Meili 默认 1000 的分页上限：默认值下
// EstimatedTotalHits 是"下限估算"（超过 1000 就不再精确），前端总数会失真。
const maxTotalHits = 1_000_000

// NewMeili 构造 Meilisearch 客户端。
func NewMeili(host, apiKey string) *Meili {
	return &Meili{
		client:      meilisearch.New(host, meilisearch.WithAPIKey(apiKey)),
		provisioned: make(map[string]string),
	}
}

// ensure 幂等开通索引：建索引（已存在则忽略）+ 设可过滤属性 + 抬高 maxTotalHits。
// Meili 对未声明 filterableAttributes 的字段过滤会直接报错，且默认分页上限使
// 总数变成估算值，故必须在写入/查询前开通。facet 键集合变化时重新应用。
func (m *Meili) ensure(index string, facetKeys []string) error {
	sig := strings.Join(sortedKeys(facetKeys), ",")

	m.mu.Lock()
	if m.provisioned[index] == sig {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	if _, err := m.client.CreateIndex(&meilisearch.IndexConfig{Uid: index, PrimaryKey: "id"}); err != nil {
		// 已存在属正常；其它错误交由下方 settings 调用暴露
		if _, statErr := m.client.GetIndex(index); statErr != nil {
			return err
		}
	}
	settings := &meilisearch.Settings{
		FilterableAttributes: sortedKeys(facetKeys),
		SortableAttributes:   []string{"published", "updated", "downloads"},
		Pagination:           &meilisearch.Pagination{MaxTotalHits: maxTotalHits},
	}
	if _, err := m.client.Index(index).GetSettingsManager().UpdateSettings(settings); err != nil {
		return err
	}

	m.mu.Lock()
	m.provisioned[index] = sig
	m.mu.Unlock()
	return nil
}

// sortedKeys 去重排序，稳定签名
func sortedKeys(keys []string) []string {
	seen := make(map[string]bool, len(keys))
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (m *Meili) Index(_ context.Context, index string, docs []search.Document) error {
	msDocs := make([]map[string]any, 0, len(docs))
	facetKeys := make([]string, 0, 8)
	for _, d := range docs {
		doc := map[string]any{"id": d.ID}
		for k, v := range d.Fields {
			doc[k] = v
		}
		for k, v := range d.Facets {
			doc[k] = v
			facetKeys = append(facetKeys, k)
		}
		msDocs = append(msDocs, doc)
	}
	if len(msDocs) == 0 {
		return nil
	}
	if err := m.ensure(index, facetKeys); err != nil {
		return err
	}
	_, err := m.client.Index(index).AddDocuments(msDocs, nil)
	return err
}

func (m *Meili) Delete(_ context.Context, index, id string) error {
	_, err := m.client.Index(index).DeleteDocument(id, nil)
	return err
}

func (m *Meili) Query(_ context.Context, index, q string, facets map[string][]string, limit, offset int) ([]string, int64, error) {
	facetKeys := make([]string, 0, len(facets))
	for k := range facets {
		facetKeys = append(facetKeys, k)
	}
	if err := m.ensure(index, facetKeys); err != nil {
		return nil, 0, err
	}

	// 同一 facet 键内 OR，键间 AND（Modrinth facets 内层 OR/外层 AND 语义）
	filters := make([]string, 0, len(facets))
	for k, vals := range facets {
		if len(vals) == 0 {
			continue
		}
		conds := make([]string, 0, len(vals))
		for _, v := range vals {
			conds = append(conds, k+" = "+v)
		}
		filters = append(filters, "("+strings.Join(conds, " OR ")+")")
	}
	req := &meilisearch.SearchRequest{
		Limit:  int64(limit),
		Offset: int64(offset),
		Filter: filters,
	}
	res, err := m.client.Index(index).Search(q, req)
	if err != nil {
		return nil, 0, err
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
	// maxTotalHits 抬高后 EstimatedTotalHits 在常规目录规模内即精确总数
	return ids, int64(res.EstimatedTotalHits), nil
}
