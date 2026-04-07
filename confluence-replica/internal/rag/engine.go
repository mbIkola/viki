package rag

import "context"

type SearchHit struct {
	PageID  string  `json:"page_id"`
	Version int     `json:"version"`
	Title   string  `json:"title"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type Citation struct {
	PageID  string `json:"page_id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
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
}

func NewEngine(r Retriever, llm LLM) *Engine {
	return &Engine{retriever: r, llm: llm}
}

func (e *Engine) Answer(ctx context.Context, query string) (Response, error) {
	hits, err := e.retriever.Retrieve(ctx, query, 8)
	if err != nil {
		return Response{}, err
	}
	if len(hits) == 0 {
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
		cites = append(cites, Citation{PageID: h.PageID, Version: h.Version, Title: h.Title})
	}
	return Response{Answer: text, Citations: cites}, nil
}
