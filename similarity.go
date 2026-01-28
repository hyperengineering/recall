package recall

import (
	"encoding/binary"
	"math"
)

// PackEmbedding converts a float32 slice to a compact binary representation.
func PackEmbedding(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// UnpackEmbedding converts a binary representation back to a float32 slice.
func UnpackEmbedding(b []byte) []float32 {
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

// ScoredLore pairs a lore entry with its similarity score.
type ScoredLore struct {
	Lore     Lore
	Score    float32
	Distance float32
}

// RankBySimilarity ranks lore entries by their similarity to a query embedding.
// Returns entries sorted by similarity (highest first).
func RankBySimilarity(query []float32, candidates []Lore, k int) []ScoredLore {
	if len(candidates) == 0 {
		return nil
	}

	scored := make([]ScoredLore, 0, len(candidates))
	for _, lore := range candidates {
		if len(lore.Embedding) == 0 {
			continue
		}
		embedding := UnpackEmbedding(lore.Embedding)
		if embedding == nil {
			continue
		}
		sim := CosineSimilarity(query, embedding)
		scored = append(scored, ScoredLore{
			Lore:     lore,
			Score:    sim,
			Distance: 1 - sim,
		})
	}

	// Sort by similarity descending (simple insertion sort for small k)
	for i := 1; i < len(scored); i++ {
		for j := i; j > 0 && scored[j].Score > scored[j-1].Score; j-- {
			scored[j], scored[j-1] = scored[j-1], scored[j]
		}
	}

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
