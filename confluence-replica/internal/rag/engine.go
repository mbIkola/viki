package rag

import "context"

type SearchHit struct {
	PageID  string  `json:"page_id"`
	Version int     `json:"version"`
	ChunkID string  `json:"chunk_id,omitempty"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type Citation struct {
	PageID  string `json:"page_id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
	ChunkID string `json:"chunk_id,omitempty"`
}

type Response struct {
	Answer    string     `json:"answer"`
	Citations []Citation `json:"citations"`
	Refused   bool       `json:"refused"`
}

type Retriever interface {
	Retrieve(ctx context.Context, query string, k int) ([]SearchHit, error)
}

type LLM interface {
	Complete(ctx context.Context, query string, ctxHits []SearchHit) (string, error)
}

type Engine struct {
	retriever Retriever
	llm       LLM
	minScore  float64
}

func NewEngine(r Retriever, llm LLM) *Engine {
	return &Engine{retriever: r, llm: llm, minScore: 0.05}
}

func (e *Engine) Answer(ctx context.Context, query string) (Response, error) {
	return e.AnswerWithTopK(ctx, query, 8)
}

func (e *Engine) AnswerWithTopK(ctx context.Context, query string, k int) (Response, error) {
	if k <= 0 {
		k = 8
	}
	hits, err := e.retriever.Retrieve(ctx, query, k)
	if err != nil {
		return Response{}, err
	}
	if len(hits) == 0 || !e.hasSufficientContext(hits) {
		return Response{
			Answer:  "I cannot answer from the local replica because no relevant indexed context was found.",
			Refused: true,
		}, nil
	}
	text, err := e.llm.Complete(ctx, query, hits)
	if err != nil {
		return Response{}, err
	}
	cites := make([]Citation, 0, len(hits))
	for _, h := range hits {
		cites = append(cites, Citation{PageID: h.PageID, Version: h.Version, Title: h.Title, ChunkID: h.ChunkID})
	}
	return Response{Answer: text, Citations: cites}, nil
}

func (e *Engine) hasSufficientContext(hits []SearchHit) bool {
	for _, h := range hits {
		if h.Score >= e.minScore && h.Snippet != "" {
			return true
		}
	}
	return false
}
