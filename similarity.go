package recall

import (
	"encoding/binary"
	"math"
	"sort"
)

// PackEmbedding converts a float32 slice to a compact binary representation.
// Deprecated: Use PackFloat32 instead.
func PackEmbedding(v []float32) []byte {
	return PackFloat32(v)
}

// PackFloat32 encodes a float32 embedding vector as a compact binary BLOB.
// Uses little-endian encoding with 4 bytes per float.
func PackFloat32(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// UnpackEmbedding converts a binary representation back to a float32 slice.
// Deprecated: Use UnpackFloat32 instead.
func UnpackEmbedding(b []byte) []float32 {
	return UnpackFloat32(b)
}

// UnpackFloat32 reconstructs a float32 vector from a packed binary BLOB.
// Returns nil if the input length is not a multiple of 4.
func UnpackFloat32(b []byte) []float32 {
	if len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns a value between -1 and 1, where 1 means identical direction.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// CosineDistance computes the cosine distance between two vectors.
// Returns a value between 0 and 2, where 0 means identical.
func CosineDistance(a, b []float32) float32 {
	return 1 - CosineSimilarity(a, b)
}

// CandidateLore represents a lore entry candidate for similarity search.
type CandidateLore struct {
	ID        string
	Embedding []float32
}

// ScoredLore pairs a lore ID with its similarity score.
type ScoredLore struct {
	ID    string
	Score float64
}

// Searcher abstracts similarity search implementations.
// Enables future swap to HNSW or other indexing strategies (NFR21).
type Searcher interface {
	Search(query []float32, candidates []CandidateLore, k int) []ScoredLore
}

// BruteForceSearcher implements Searcher using exhaustive cosine similarity.
type BruteForceSearcher struct{}

// Search ranks candidates by cosine similarity to the query, returning top-k results.
// Results are sorted by score descending (highest similarity first).
// If k <= 0 or k > len(candidates), returns all matching candidates.
func (s *BruteForceSearcher) Search(query []float32, candidates []CandidateLore, k int) []ScoredLore {
	if len(candidates) == 0 {
		return []ScoredLore{}
	}

	scored := make([]ScoredLore, 0, len(candidates))
	for _, c := range candidates {
		// Skip empty or mismatched dimension embeddings
		if len(c.Embedding) == 0 || len(c.Embedding) != len(query) {
			continue
		}
		sim := CosineSimilarity(query, c.Embedding)
		scored = append(scored, ScoredLore{
			ID:    c.ID,
			Score: float64(sim),
		})
	}

	// Sort by score descending using sort.Slice
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Apply top-k limit
	if k > 0 && len(scored) > k {
		scored = scored[:k]
	}

	return scored
}

// NormalizeEmbedding normalizes a vector to unit length.
func NormalizeEmbedding(v []float32) []float32 {
	var norm float32
	for _, x := range v {
		norm += x * x
	}
	if norm == 0 {
		return v
	}

	norm = float32(math.Sqrt(float64(norm)))
	result := make([]float32, len(v))
	for i, x := range v {
		result[i] = x / norm
	}
	return result
}
