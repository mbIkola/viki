package ingest

import (
	"context"
	"testing"

	"confluence-replica/internal/store"
)

type fakeEmb struct{}

func (f fakeEmb) Embed(_ context.Context, _ string) ([]float32, error) {
	return []float32{0.1, 0.2}, nil
}

func TestFillChunkEmbeddings(t *testing.T) {
	s := &Service{emb: fakeEmb{}}
	chunks := []store.Chunk{{ChunkID: "1", ChunkText: "hello"}, {ChunkID: "2", ChunkText: "world"}}
	if err := s.fillChunkEmbeddings(context.Background(), chunks); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for _, c := range chunks {
		if len(c.Embedding) != 2 {
			t.Fatalf("expected embedding for chunk %s", c.ChunkID)
		}
	}
}
