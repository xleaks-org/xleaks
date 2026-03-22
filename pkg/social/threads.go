package social

import (
	"bytes"
	"context"
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
func (s *PostService) GetThread(ctx context.Context, postCID []byte) (*ThreadNode, error) {
	// Get the root post.
	rootRow, err := s.storage.GetPost(postCID)
	if err != nil {
		return nil, fmt.Errorf("get root post: %w", err)
	}
	if rootRow == nil {
		return nil, fmt.Errorf("post not found")
	}

	rootPost := postRowToProto(rootRow)
	likeCount, _ := s.storage.GetReactionCount(postCID)

	root := &ThreadNode{
		Post:      rootPost,
		LikeCount: likeCount,
	}

	// Recursively build the thread tree.
	if err := s.buildChildren(ctx, root); err != nil {
		return nil, fmt.Errorf("build thread tree: %w", err)
	}

	return root, nil
}

// buildChildren recursively loads replies and builds the thread tree.
func (s *PostService) buildChildren(ctx context.Context, node *ThreadNode) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	replies, err := s.storage.GetThread(node.Post.Id)
	if err != nil {
		return fmt.Errorf("get replies: %w", err)
	}

	node.ReplyCount = len(replies)

	for _, replyRow := range replies {
		replyPost := postRowToProto(&replyRow)
		likeCount, _ := s.storage.GetReactionCount(replyRow.CID)

		child := &ThreadNode{
			Post:      replyPost,
			LikeCount: likeCount,
		}

		if err := s.buildChildren(ctx, child); err != nil {
			return err
		}

		node.Children = append(node.Children, child)
	}

	return nil
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
