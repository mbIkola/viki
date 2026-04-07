-- Make embedding column model-agnostic (variable dimension) and track dim explicitly.
ALTER TABLE chunk_embeddings
  ALTER COLUMN embedding TYPE vector
  USING embedding::vector;

ALTER TABLE chunk_embeddings
  ADD COLUMN IF NOT EXISTS embedding_dim INTEGER;

UPDATE chunk_embeddings
SET embedding_dim = vector_dims(embedding)
WHERE embedding_dim IS NULL;

ALTER TABLE chunk_embeddings
  ALTER COLUMN embedding_dim SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_chunk_embeddings_dim ON chunk_embeddings (embedding_dim);
