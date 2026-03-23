package social

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"github.com/xleaks-org/xleaks/pkg/storage"
	pb "github.com/xleaks-org/xleaks/proto/gen"
)

// ThreadNode represents a node in a comment thread tree.
type ThreadNode struct {
	Post       *pb.Post
	Children   []*ThreadNode
	ReplyCount int
	LikeCount  int
}

// GetThread retrieves a post and all its replies, assembled into a tree structure.
// It batch-fetches all descendant replies and reaction counts to avoid N+1 queries.
func (s *PostService) GetThread(ctx context.Context, postCID []byte) (*ThreadNode, error) {
	// Get the root post.
	rootRow, err := s.storage.GetPost(postCID)
	if err != nil {
		return nil, fmt.Errorf("get root post: %w", err)
	}
	if rootRow == nil {
		return nil, fmt.Errorf("post not found")
	}

	// Batch-fetch all descendant replies in one query.
	allReplies, err := s.storage.GetAllDescendantReplies(postCID)
	if err != nil {
		return nil, fmt.Errorf("batch fetch replies: %w", err)
	}

	// Collect all CIDs (root + replies) for batch reaction count fetch.
	allCIDs := make([][]byte, 0, 1+len(allReplies))
	allCIDs = append(allCIDs, postCID)
	for i := range allReplies {
		allCIDs = append(allCIDs, allReplies[i].CID)
	}

	// Batch-fetch all reaction counts.
	reactionCounts, err := s.storage.GetReactionCountsBatch(allCIDs)
	if err != nil {
		return nil, fmt.Errorf("batch fetch reaction counts: %w", err)
	}

	// Build a map from parent CID hex -> child rows for O(1) lookup.
	childrenMap := make(map[string][]storage.PostRow)
	for _, reply := range allReplies {
		parentHex := hex.EncodeToString(reply.ReplyTo)
		childrenMap[parentHex] = append(childrenMap[parentHex], reply)
	}

	// Build the root node.
	rootPost := postRowToProto(rootRow)
	rootHex := hex.EncodeToString(postCID)
	rc := reactionCounts[rootHex]

	root := &ThreadNode{
		Post:      rootPost,
		LikeCount: rc.Likes,
	}

	// Recursively assemble the tree using the pre-fetched data.
	assembleChildren(root, childrenMap, reactionCounts)

	return root, nil
}

// assembleChildren recursively builds the thread tree from pre-fetched data.
func assembleChildren(node *ThreadNode, childrenMap map[string][]storage.PostRow, reactionCounts map[string]storage.ReactionCounts) {
	cidHex := hex.EncodeToString(node.Post.Id)
	replies := childrenMap[cidHex]
	node.ReplyCount = len(replies)

	for i := range replies {
		replyPost := postRowToProto(&replies[i])
		replyHex := hex.EncodeToString(replies[i].CID)
		rc := reactionCounts[replyHex]

		child := &ThreadNode{
			Post:      replyPost,
			LikeCount: rc.Likes,
		}

		assembleChildren(child, childrenMap, reactionCounts)
		node.Children = append(node.Children, child)
	}
}

// postRowToProto converts a storage.PostRow to a pb.Post protobuf message.
func postRowToProto(row *storage.PostRow) *pb.Post {
	post := &pb.Post{
		Id:        row.CID,
		Author:    row.Author,
		Content:   row.Content,
		Timestamp: uint64(row.Timestamp),
		Signature: row.Signature,
	}
	if len(row.ReplyTo) > 0 && !bytes.Equal(row.ReplyTo, []byte{}) {
		post.ReplyTo = row.ReplyTo
	}
	if len(row.RepostOf) > 0 && !bytes.Equal(row.RepostOf, []byte{}) {
		post.RepostOf = row.RepostOf
	}
	return post
}
