package merkle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const (
	// DefaultChunkSize is the default chunk size for splitting files (1MB)
	DefaultChunkSize = 1024 * 1024
	// MinChunkSize is the minimum allowed chunk size (64KB)
	MinChunkSize = 64 * 1024
	// MaxChunkSize is the maximum allowed chunk size (10MB)
	MaxChunkSize = 10 * 1024 * 1024
)

// Tree represents a Merkle tree for file verification
type Tree struct {
	Root      string   `json:"root"`
	Chunks    []Chunk  `json:"chunks"`
	ChunkSize int      `json:"chunk_size"`
	FileSize  int64    `json:"file_size"`
	Levels    [][]Node `json:"-"` // Internal tree structure
}

// Chunk represents a file chunk with its hash
type Chunk struct {
	Index  int    `json:"index"`
	Offset int64  `json:"offset"`
	Size   int    `json:"size"`
	Hash   string `json:"hash"`
}

// Node represents a node in the Merkle tree
type Node struct {
	Hash   string
	Left   *Node
	Right  *Node
	IsLeaf bool
	Index  int
}

// BuildTreeFromFile builds a Merkle tree from a file
func BuildTreeFromFile(filePath string, chunkSize int) (*Tree, error) {
	if chunkSize < MinChunkSize || chunkSize > MaxChunkSize {
		chunkSize = DefaultChunkSize
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	tree := &Tree{
		ChunkSize: chunkSize,
		FileSize:  fileInfo.Size(),
		Chunks:    make([]Chunk, 0),
		Levels:    make([][]Node, 0),
	}

	// Read file in chunks and build leaf nodes
	leafNodes := make([]Node, 0)
	buffer := make([]byte, chunkSize)
	offset := int64(0)
	index := 0

	for {
		n, err := file.Read(buffer)
		if n > 0 {
			// Calculate hash for this chunk
			hash := sha256.Sum256(buffer[:n])
			hashStr := hex.EncodeToString(hash[:])

			// Add chunk info
			chunk := Chunk{
				Index:  index,
				Offset: offset,
				Size:   n,
				Hash:   hashStr,
			}
			tree.Chunks = append(tree.Chunks, chunk)

			// Add leaf node
			leafNodes = append(leafNodes, Node{
				Hash:   hashStr,
				IsLeaf: true,
				Index:  index,
			})

			offset += int64(n)
			index++
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
	}

	// Build the Merkle tree
	tree.Levels = append(tree.Levels, leafNodes)
	tree.Root = tree.buildTree(leafNodes)

	return tree, nil
}

// buildTree builds the Merkle tree from leaf nodes
func (t *Tree) buildTree(leaves []Node) string {
	if len(leaves) == 0 {
		return ""
	}
	if len(leaves) == 1 {
		return leaves[0].Hash
	}

	currentLevel := leaves
	
	for len(currentLevel) > 1 {
		nextLevel := make([]Node, 0)
		
		for i := 0; i < len(currentLevel); i += 2 {
			left := &currentLevel[i]
			
			var right *Node
			var combinedHash string
			
			if i+1 < len(currentLevel) {
				right = &currentLevel[i+1]
				// Combine hashes of left and right nodes
				combined := left.Hash + right.Hash
				hash := sha256.Sum256([]byte(combined))
				combinedHash = hex.EncodeToString(hash[:])
			} else {
				// Odd number of nodes, duplicate the last one
				combined := left.Hash + left.Hash
				hash := sha256.Sum256([]byte(combined))
				combinedHash = hex.EncodeToString(hash[:])
				right = left
			}
			
			node := Node{
				Hash:   combinedHash,
				Left:   left,
				Right:  right,
				IsLeaf: false,
			}
			
			nextLevel = append(nextLevel, node)
		}
		
		t.Levels = append(t.Levels, nextLevel)
		currentLevel = nextLevel
	}
	
	return currentLevel[0].Hash
}

// VerifyChunk verifies a specific chunk against the Merkle tree
func (t *Tree) VerifyChunk(chunkIndex int, chunkData []byte) (bool, error) {
	if chunkIndex < 0 || chunkIndex >= len(t.Chunks) {
		return false, fmt.Errorf("invalid chunk index: %d", chunkIndex)
	}

	// Calculate hash of provided chunk data
	hash := sha256.Sum256(chunkData)
	hashStr := hex.EncodeToString(hash[:])

	// Compare with stored hash
	return hashStr == t.Chunks[chunkIndex].Hash, nil
}

// GetProof returns the Merkle proof for a specific chunk
func (t *Tree) GetProof(chunkIndex int) ([]string, error) {
	if chunkIndex < 0 || chunkIndex >= len(t.Chunks) {
		return nil, fmt.Errorf("invalid chunk index: %d", chunkIndex)
	}

	proof := make([]string, 0)
	currentIndex := chunkIndex

	// Traverse up the tree collecting sibling hashes
	for level := 0; level < len(t.Levels)-1; level++ {
		currentLevel := t.Levels[level]
		siblingIndex := currentIndex ^ 1 // XOR to get sibling index

		if siblingIndex < len(currentLevel) {
			proof = append(proof, currentLevel[siblingIndex].Hash)
		} else if currentIndex < len(currentLevel) {
			// No sibling, duplicate current node
			proof = append(proof, currentLevel[currentIndex].Hash)
		}

		currentIndex = currentIndex / 2
	}

	return proof, nil
}

// VerifyProof verifies a Merkle proof for a chunk
func VerifyProof(chunkHash string, chunkIndex int, proof []string, rootHash string) bool {
	currentHash := chunkHash
	currentIndex := chunkIndex

	for _, siblingHash := range proof {
		var combined string
		if currentIndex%2 == 0 {
			// Current node is left child
			combined = currentHash + siblingHash
		} else {
			// Current node is right child
			combined = siblingHash + currentHash
		}

		hash := sha256.Sum256([]byte(combined))
		currentHash = hex.EncodeToString(hash[:])
		currentIndex = currentIndex / 2
	}

	return currentHash == rootHash
}

// ChunkReader provides a reader for a specific chunk
type ChunkReader struct {
	file   *os.File
	chunk  *Chunk
	offset int64
	limit  int64
}

// NewChunkReader creates a reader for a specific chunk
func NewChunkReader(filePath string, chunk *Chunk) (*ChunkReader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	if _, err := file.Seek(chunk.Offset, 0); err != nil {
		file.Close()
		return nil, err
	}

	return &ChunkReader{
		file:   file,
		chunk:  chunk,
		offset: 0,
		limit:  int64(chunk.Size),
	}, nil
}

// Read implements io.Reader
func (cr *ChunkReader) Read(p []byte) (n int, err error) {
	if cr.offset >= cr.limit {
		return 0, io.EOF
	}

	remaining := cr.limit - cr.offset
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	n, err = cr.file.Read(p)
	cr.offset += int64(n)
	return n, err
}

// Close closes the underlying file
func (cr *ChunkReader) Close() error {
	return cr.file.Close()
}

// ChunkWriter provides a writer for updating specific chunks
type ChunkWriter struct {
	filePath  string
	tree      *Tree
	chunkData map[int][]byte
}

// NewChunkWriter creates a writer for updating chunks
func NewChunkWriter(filePath string, tree *Tree) *ChunkWriter {
	return &ChunkWriter{
		filePath:  filePath,
		tree:      tree,
		chunkData: make(map[int][]byte),
	}
}

// WriteChunk stages a chunk for writing
func (cw *ChunkWriter) WriteChunk(index int, data []byte) error {
	if index < 0 || index >= len(cw.tree.Chunks) {
		return fmt.Errorf("invalid chunk index: %d", index)
	}

	// Verify chunk size
	expectedSize := cw.tree.Chunks[index].Size
	if len(data) != expectedSize {
		return fmt.Errorf("chunk size mismatch: expected %d, got %d", expectedSize, len(data))
	}

	// Verify chunk hash
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:])
	if hashStr != cw.tree.Chunks[index].Hash {
		return fmt.Errorf("chunk hash mismatch")
	}

	cw.chunkData[index] = data
	return nil
}

// Commit writes all staged chunks to the file
func (cw *ChunkWriter) Commit() error {
	file, err := os.OpenFile(cw.filePath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	for index, data := range cw.chunkData {
		chunk := cw.tree.Chunks[index]
		if _, err := file.Seek(chunk.Offset, 0); err != nil {
			return fmt.Errorf("failed to seek to chunk %d: %w", index, err)
		}

		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("failed to write chunk %d: %w", index, err)
		}
	}

	return nil
}