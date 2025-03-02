// SPDX-License-Identifier: Apache-2.0

package gitinterface

import (
	"fmt"
	"testing"

	"github.com/gittuf/gittuf/internal/third_party/go-git"
	"github.com/gittuf/gittuf/internal/third_party/go-git/plumbing"
	"github.com/gittuf/gittuf/internal/third_party/go-git/plumbing/filemode"
	"github.com/gittuf/gittuf/internal/third_party/go-git/plumbing/object"
	"github.com/gittuf/gittuf/internal/third_party/go-git/storage/memory"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/stretchr/testify/assert"
)

func TestGetCommitFilePaths(t *testing.T) {
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatal(err)
	}

	emptyBlobHash := EmptyBlob()

	tests := map[string]struct {
		treeEntries   []object.TreeEntry
		expectedPaths []string
	}{
		"one file": {
			treeEntries: []object.TreeEntry{
				{
					Name: "a",
					Mode: filemode.Regular,
					Hash: emptyBlobHash,
				},
			},
			expectedPaths: []string{"a"},
		},
		"multiple files": {
			treeEntries: []object.TreeEntry{
				{
					Name: "a",
					Mode: filemode.Regular,
					Hash: emptyBlobHash,
				},
				{
					Name: "b",
					Mode: filemode.Regular,
					Hash: emptyBlobHash,
				},
			},
			expectedPaths: []string{"a", "b"},
		},
		"no files": {
			treeEntries:   []object.TreeEntry{},
			expectedPaths: []string{},
		},
	}

	for name, test := range tests {
		WriteBlob(repo, []byte{}) //nolint: errcheck
		treeHash, err := WriteTree(repo, test.treeEntries)
		if err != nil {
			t.Fatal(err)
		}

		c := CreateCommitObject(testGitConfig, treeHash, plumbing.ZeroHash, "Test commit", testClock)
		commitID, err := WriteCommit(repo, c)
		if err != nil {
			t.Fatal(err)
		}
		commit, err := repo.CommitObject(commitID)
		if err != nil {
			t.Fatal(err)
		}

		paths, err := GetCommitFilePaths(commit)
		assert.Nil(t, err, fmt.Sprintf("unexpected error in test %s", name))
		assert.Equal(t, test.expectedPaths, paths, fmt.Sprintf("unexpected list of files received: expected %v, got %v in test %s", test.expectedPaths, paths, name))
	}
}

func TestGetDiffFilePaths(t *testing.T) {
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatal(err)
	}

	blobIDs := []plumbing.Hash{}
	for i := 0; i < 3; i++ {
		blobID, err := WriteBlob(repo, []byte(fmt.Sprintf("%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		blobIDs = append(blobIDs, blobID)
	}

	t.Run("modify single file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]}})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{{Name: "a", Mode: filemode.Regular, Hash: blobIDs[1]}})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, plumbing.ZeroHash, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commitA, err := repo.CommitObject(cAID)
		if err != nil {
			t.Fatal(err)
		}
		commitB, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetDiffFilePaths(commitA, commitB)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a"}, diffs)
	})

	t.Run("rename single file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]}})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{{Name: "b", Mode: filemode.Regular, Hash: blobIDs[0]}})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, plumbing.ZeroHash, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commitA, err := repo.CommitObject(cAID)
		if err != nil {
			t.Fatal(err)
		}
		commitB, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetDiffFilePaths(commitA, commitB)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a", "b"}, diffs)
	})

	t.Run("swap two files around", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[1]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, plumbing.ZeroHash, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commitA, err := repo.CommitObject(cAID)
		if err != nil {
			t.Fatal(err)
		}
		commitB, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetDiffFilePaths(commitA, commitB)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a", "b"}, diffs)
	})

	t.Run("create new file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, plumbing.ZeroHash, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commitA, err := repo.CommitObject(cAID)
		if err != nil {
			t.Fatal(err)
		}
		commitB, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetDiffFilePaths(commitA, commitB)
		assert.Nil(t, err)
		assert.Equal(t, []string{"b"}, diffs)
	})

	t.Run("delete file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, plumbing.ZeroHash, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commitA, err := repo.CommitObject(cAID)
		if err != nil {
			t.Fatal(err)
		}
		commitB, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetDiffFilePaths(commitA, commitB)
		assert.Nil(t, err)
		assert.Equal(t, []string{"b"}, diffs)
	})

	t.Run("modify file and create new file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[2]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, plumbing.ZeroHash, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commitA, err := repo.CommitObject(cAID)
		if err != nil {
			t.Fatal(err)
		}
		commitB, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetDiffFilePaths(commitA, commitB)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a", "b"}, diffs)
	})
}

func TestGetFilePathsChangedByCommit(t *testing.T) {
	repo, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		t.Fatal(err)
	}

	blobIDs := []plumbing.Hash{}
	for i := 0; i < 3; i++ {
		blobID, err := WriteBlob(repo, []byte(fmt.Sprintf("%d", i)))
		if err != nil {
			t.Fatal(err)
		}
		blobIDs = append(blobIDs, blobID)
	}

	t.Run("modify single file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]}})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{{Name: "a", Mode: filemode.Regular, Hash: blobIDs[1]}})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, cAID, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commit, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetFilePathsChangedByCommit(repo, commit)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a"}, diffs)
	})

	t.Run("rename single file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]}})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{{Name: "b", Mode: filemode.Regular, Hash: blobIDs[0]}})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, cAID, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commit, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetFilePathsChangedByCommit(repo, commit)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a", "b"}, diffs)
	})

	t.Run("swap two files around", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[1]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, cAID, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commit, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetFilePathsChangedByCommit(repo, commit)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a", "b"}, diffs)
	})

	t.Run("create new file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, cAID, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commit, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetFilePathsChangedByCommit(repo, commit)
		assert.Nil(t, err)
		assert.Equal(t, []string{"b"}, diffs)
	})

	t.Run("delete file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, cAID, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commit, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetFilePathsChangedByCommit(repo, commit)
		assert.Nil(t, err)
		assert.Equal(t, []string{"b"}, diffs)
	})

	t.Run("modify file and create new file", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		treeB, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[2]},
			{Name: "b", Mode: filemode.Regular, Hash: blobIDs[1]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		cB := CreateCommitObject(testGitConfig, treeB, cAID, "Test commit", testClock)
		cBID, err := WriteCommit(repo, cB)
		if err != nil {
			t.Fatal(err)
		}

		commit, err := repo.CommitObject(cBID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetFilePathsChangedByCommit(repo, commit)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a", "b"}, diffs)
	})

	t.Run("no parent", func(t *testing.T) {
		treeA, err := WriteTree(repo, []object.TreeEntry{
			{Name: "a", Mode: filemode.Regular, Hash: blobIDs[0]},
		})
		if err != nil {
			t.Fatal(err)
		}

		cA := CreateCommitObject(testGitConfig, treeA, plumbing.ZeroHash, "Test commit", testClock)
		cAID, err := WriteCommit(repo, cA)
		if err != nil {
			t.Fatal(err)
		}

		commit, err := repo.CommitObject(cAID)
		if err != nil {
			t.Fatal(err)
		}

		diffs, err := GetFilePathsChangedByCommit(repo, commit)
		assert.Nil(t, err)
		assert.Equal(t, []string{"a"}, diffs)
	})
}
