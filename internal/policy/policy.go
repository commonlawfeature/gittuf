// SPDX-License-Identifier: Apache-2.0

package policy

import (
	"errors"

	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gittuf/gittuf/internal/gitinterface"
	"github.com/gittuf/gittuf/internal/rsl"
	"github.com/gittuf/gittuf/internal/signerverifier"
	"github.com/gittuf/gittuf/internal/signerverifier/dsse"
	"github.com/gittuf/gittuf/internal/third_party/go-git"
	"github.com/gittuf/gittuf/internal/third_party/go-git/plumbing"
	"github.com/gittuf/gittuf/internal/third_party/go-git/plumbing/filemode"
	"github.com/gittuf/gittuf/internal/third_party/go-git/plumbing/object"
	"github.com/gittuf/gittuf/internal/tuf"
	sslibdsse "github.com/secure-systems-lab/go-securesystemslib/dsse"
)

const (
	// PolicyRef defines the Git namespace used for gittuf policies.
	PolicyRef = "refs/gittuf/policy"

	// PolicyStagingRef defines the Git namespace used as a staging area when creating or updating gittuf policies.
	PolicyStagingRef = "refs/gittuf/policy-staging"

	// RootRoleName defines the expected name for the gittuf root of trust.
	RootRoleName = "root"

	// TargetsRoleName defines the expected name for the top level gittuf policy file.
	TargetsRoleName = "targets"

	// DefaultCommitMessage defines the fallback message to use when updating the policy ref if an action specific message is unavailable.
	DefaultCommitMessage = "Update policy state"

	rootPublicKeysTreeEntryName = "keys"
	metadataTreeEntryName       = "metadata"
)

var (
	ErrMetadataNotFound           = errors.New("unable to find requested metadata file; has it been initialized?")
	ErrInvalidPolicyTree          = errors.New("invalid policy tree structure")
	ErrDanglingDelegationMetadata = errors.New("unreachable targets metadata found")
	ErrNotRSLEntry                = errors.New("RSL entry expected, annotation found instead")
	ErrDelegationNotFound         = errors.New("required delegation entry not found")
)

var ErrPolicyExists = errors.New("cannot initialize Policy namespace as it exists already")

// InitializeNamespace creates a git ref for the policy. Initially, the entry
// has a zero hash.
func InitializeNamespace(repo *git.Repository) error {
	for _, name := range []string{PolicyRef /*, PolicyStagingRef*/} {
		if ref, err := repo.Reference(plumbing.ReferenceName(name), true); err != nil {
			if !errors.Is(err, plumbing.ErrReferenceNotFound) {
				return err
			}
		} else if !ref.Hash().IsZero() {
			return ErrPolicyExists
		}
	}

	// Disable PolicyStagingRef until it is actually used
	// https://github.com/gittuf/gittuf/issues/45
	// if err := repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(PolicyStagingRef), plumbing.ZeroHash)); err != nil {
	// 	return err
	// }

	return repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(PolicyRef), plumbing.ZeroHash))
}

// State contains the full set of metadata and root keys present in a policy
// state.
type State struct {
	RootEnvelope        *sslibdsse.Envelope
	TargetsEnvelope     *sslibdsse.Envelope
	DelegationEnvelopes map[string]*sslibdsse.Envelope
	RootPublicKeys      []*tuf.Key
}

// LoadState returns the State of the repository's policy corresponding to the
// rslEntryID.
func LoadState(ctx context.Context, repo *git.Repository, rslEntryID plumbing.Hash) (*State, error) {
	e, err := rsl.GetEntry(repo, rslEntryID)
	if err != nil {
		return nil, err
	}

	return LoadStateForEntry(ctx, repo, e)
}

// LoadCurrentState returns the State corresponding to the repository's current
// active policy.
func LoadCurrentState(ctx context.Context, repo *git.Repository) (*State, error) {
	e, _, err := rsl.GetLatestReferenceEntryForRef(repo, PolicyRef)
	if err != nil {
		return nil, err
	}

	return LoadStateForEntry(ctx, repo, e)
}

// LoadStateForEntry returns the State for a specified RSL entry for the policy
// namespace.
func LoadStateForEntry(ctx context.Context, repo *git.Repository, e rsl.Entry) (*State, error) {
	entry, ok := e.(*rsl.ReferenceEntry)
	if !ok {
		return nil, ErrNotRSLEntry
	}

	if entry.RefName != PolicyRef {
		return nil, rsl.ErrRSLEntryDoesNotMatchRef
	}

	policyCommit, err := repo.CommitObject(entry.TargetID)
	if err != nil {
		return nil, err
	}

	policyRootTree, err := repo.TreeObject(policyCommit.TreeHash)
	if err != nil {
		return nil, err
	}

	if len(policyRootTree.Entries) > 2 {
		return nil, ErrInvalidPolicyTree
	}

	var (
		metadataTreeID plumbing.Hash
		keysTreeID     plumbing.Hash
	)

	for _, e := range policyRootTree.Entries {
		switch e.Name {
		case metadataTreeEntryName:
			metadataTreeID = e.Hash
		case rootPublicKeysTreeEntryName:
			keysTreeID = e.Hash
		default:
			return nil, ErrInvalidPolicyTree
		}
	}

	state := &State{}

	metadataTree, err := repo.TreeObject(metadataTreeID)
	if err != nil {
		return nil, err
	}

	keysTree, err := repo.TreeObject(keysTreeID)
	if err != nil {
		return nil, err
	}

	for _, entry := range metadataTree.Entries {
		contents, err := gitinterface.ReadBlob(repo, entry.Hash)
		if err != nil {
			return nil, err
		}

		env := &sslibdsse.Envelope{}
		if err := json.Unmarshal(contents, env); err != nil {
			return nil, err
		}

		switch entry.Name {
		case fmt.Sprintf("%s.json", RootRoleName):
			state.RootEnvelope = env
		case fmt.Sprintf("%s.json", TargetsRoleName):
			state.TargetsEnvelope = env
		default:
			if state.DelegationEnvelopes == nil {
				state.DelegationEnvelopes = map[string]*sslibdsse.Envelope{}
			}

			state.DelegationEnvelopes[strings.TrimSuffix(entry.Name, ".json")] = env
		}
	}

	for _, entry := range keysTree.Entries {
		contents, err := gitinterface.ReadBlob(repo, entry.Hash)
		if err != nil {
			return nil, err
		}

		key, err := tuf.LoadKeyFromBytes(contents)
		if err != nil {
			return nil, err
		}

		if state.RootPublicKeys == nil {
			state.RootPublicKeys = []*tuf.Key{}
		}

		state.RootPublicKeys = append(state.RootPublicKeys, key)
	}

	if err := state.Verify(ctx); err != nil {
		return nil, err
	}

	return state, nil
}

// GetStateForCommit scans the RSL to identify the first time a commit was seen
// in the repository. The policy preceding that RSL entry is returned as the
// State to be used for verifying the commit's signature. If the commit hasn't
// been seen in the repository previously, no policy state is returned. Also, no
// error is returned. Identifying the policy in this case is left to the calling
// workflow.
func GetStateForCommit(ctx context.Context, repo *git.Repository, commit *object.Commit) (*State, error) {
	firstSeenEntry, _, err := rsl.GetFirstReferenceEntryForCommit(repo, commit)
	if err != nil {
		if errors.Is(err, rsl.ErrNoRecordOfCommit) {
			return nil, nil
		}
		return nil, err
	}

	commitPolicyEntry, _, err := rsl.GetLatestReferenceEntryForRefBefore(repo, PolicyRef, firstSeenEntry.ID)
	if err != nil {
		return nil, err
	}

	return LoadStateForEntry(ctx, repo, commitPolicyEntry)
}

// PublicKeys returns all the public keys associated with a state.
func (s *State) PublicKeys() (map[string]*tuf.Key, error) {
	allKeys := map[string]*tuf.Key{}

	// Add root keys
	for _, key := range s.RootPublicKeys {
		key := key
		allKeys[key.KeyID] = key
	}

	// Add keys from the root metadata
	rootMetadata, err := s.GetRootMetadata()
	if err != nil {
		return nil, err
	}
	for keyID, key := range rootMetadata.Keys {
		key := key
		allKeys[keyID] = key
	}

	// Add keys from top level targets metadata
	if s.TargetsEnvelope == nil {
		// Early states where this hasn't been initialized yet
		return nil, err
	}
	targetsMetadata, err := s.GetTargetsMetadata(TargetsRoleName)
	if err != nil {
		return nil, err
	}
	for keyID, key := range targetsMetadata.Delegations.Keys {
		key := key
		allKeys[keyID] = key
	}

	// Add keys from delegated targets metadata
	for roleName := range s.DelegationEnvelopes {
		delegatedMetadata, err := s.GetTargetsMetadata(roleName)
		if err != nil {
			return nil, err
		}
		for keyID, key := range delegatedMetadata.Delegations.Keys {
			key := key
			allKeys[keyID] = key
		}
	}

	return allKeys, nil
}

// FindAuthorizedSigningKeyIDs traverses the policy metadata to identify the
// keys trusted to sign for the specified role.
func (s *State) FindAuthorizedSigningKeyIDs(ctx context.Context, roleName string) ([]string, error) {
	if err := s.Verify(ctx); err != nil {
		return nil, err
	}

	rootMetadata, err := s.GetRootMetadata()
	if err != nil {
		return nil, err
	}

	if roleName == RootRoleName {
		return rootMetadata.Roles[RootRoleName].KeyIDs, nil
	}

	if roleName == TargetsRoleName {
		if _, ok := rootMetadata.Roles[TargetsRoleName]; !ok {
			return nil, ErrDelegationNotFound
		}

		return rootMetadata.Roles[TargetsRoleName].KeyIDs, nil
	}

	entry, err := s.findDelegationEntry(roleName)
	if err != nil {
		return nil, err
	}

	return entry.KeyIDs, nil
}

// FindPublicKeysForPath identifies the trusted keys for the path. If the path
// protected in gittuf policy, the trusted keys are returned.
func (s *State) FindPublicKeysForPath(ctx context.Context, path string) ([]*tuf.Key, error) {
	if err := s.Verify(ctx); err != nil {
		return nil, err
	}

	targetsMetadata, err := s.GetTargetsMetadata(TargetsRoleName)
	if err != nil {
		return nil, err
	}

	allPublicKeys := targetsMetadata.Delegations.Keys
	delegationsQueue := targetsMetadata.Delegations.Roles

	trustedKeys := []*tuf.Key{}
	for {
		if len(delegationsQueue) <= 1 {
			return trustedKeys, nil
		}

		delegation := delegationsQueue[0]
		delegationsQueue = delegationsQueue[1:]

		if delegation.Matches(path) {
			for _, keyID := range delegation.KeyIDs {
				key := allPublicKeys[keyID]
				trustedKeys = append(trustedKeys, key)
			}

			if s.HasTargetsRole(delegation.Name) {
				delegatedMetadata, err := s.GetTargetsMetadata(delegation.Name)
				if err != nil {
					return nil, err
				}
				for keyID, key := range delegatedMetadata.Delegations.Keys {
					allPublicKeys[keyID] = key
				}

				if delegation.Terminating {
					// Remove other delegations from the queue
					delegationsQueue = delegatedMetadata.Delegations.Roles
				} else {
					// Depth first, so newly discovered delegations go first
					// Also, we skip the allow-rule, so we don't include the
					// last element in the delegatedMetadata list.
					delegationsQueue = append(delegatedMetadata.Delegations.Roles[:len(delegatedMetadata.Delegations.Roles)-1], delegationsQueue...)
				}
			}
		}
	}
}

// Verify performs a self-contained verification of all the metadata in the
// State starting from the Root. Any metadata that is unreachable in the
// delegations graph returns an error.
func (s *State) Verify(ctx context.Context) error {
	rootVerifiers := []sslibdsse.Verifier{}
	for _, k := range s.RootPublicKeys {
		sv, err := signerverifier.NewSignerVerifierFromTUFKey(k)
		if err != nil {
			return err
		}

		rootVerifiers = append(rootVerifiers, sv)
	}
	if err := dsse.VerifyEnvelope(ctx, s.RootEnvelope, rootVerifiers, len(rootVerifiers)); err != nil {
		return err
	}

	if s.TargetsEnvelope == nil {
		return nil
	}

	rootMetadata := &tuf.RootMetadata{}
	rootContents, err := s.RootEnvelope.DecodeB64Payload()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(rootContents, rootMetadata); err != nil {
		return err
	}

	targetsVerifiers := []sslibdsse.Verifier{}
	for _, keyID := range rootMetadata.Roles[TargetsRoleName].KeyIDs {
		key := rootMetadata.Keys[keyID]
		sv, err := signerverifier.NewSignerVerifierFromTUFKey(key)
		if err != nil {
			return err
		}

		targetsVerifiers = append(targetsVerifiers, sv)
	}
	if err := dsse.VerifyEnvelope(ctx, s.TargetsEnvelope, targetsVerifiers, rootMetadata.Roles[TargetsRoleName].Threshold); err != nil {
		return err
	}

	if len(s.DelegationEnvelopes) == 0 {
		return nil
	}

	delegationEnvelopes := map[string]*sslibdsse.Envelope{}
	for k, v := range s.DelegationEnvelopes {
		delegationEnvelopes[k] = v
	}

	targetsMetadata := &tuf.TargetsMetadata{}
	targetsContents, err := s.TargetsEnvelope.DecodeB64Payload()
	if err != nil {
		return err
	}
	if err := json.Unmarshal(targetsContents, targetsMetadata); err != nil {
		return err
	}

	if err := targetsMetadata.Validate(); err != nil {
		return err
	}

	// Note: If targetsMetadata.Delegations == nil while delegationEnvelopes is
	// not empty, we probably want to error out. This should panic.
	delegationKeys := targetsMetadata.Delegations.Keys
	delegationsQueue := targetsMetadata.Delegations.Roles

	// We can likely process top level targets and all delegated envelopes in
	// the loop below by combining the two but this separated model seems easier
	// to reason about. Else, we define a custom starting delegation from root
	// to targets in the queue and start this loop from there.

	for {
		if len(delegationsQueue) == 0 {
			break
		}

		delegation := delegationsQueue[0]
		delegationsQueue = delegationsQueue[1:]

		delegationEnvelope, ok := delegationEnvelopes[delegation.Name]
		if !ok {
			// Delegation does not have an envelope to verify
			continue
		}
		delete(delegationEnvelopes, delegation.Name)

		delegationVerifiers := make([]sslibdsse.Verifier, 0, len(delegation.KeyIDs))
		for _, keyID := range delegation.KeyIDs {
			key := delegationKeys[keyID]
			sv, err := signerverifier.NewSignerVerifierFromTUFKey(key)
			if err != nil {
				return err
			}

			delegationVerifiers = append(delegationVerifiers, sv)
		}

		if err := dsse.VerifyEnvelope(ctx, delegationEnvelope, delegationVerifiers, delegation.Threshold); err != nil {
			return err
		}

		delegationMetadata := &tuf.TargetsMetadata{}
		delegationContents, err := delegationEnvelope.DecodeB64Payload()
		if err != nil {
			return err
		}
		if err := json.Unmarshal(delegationContents, delegationMetadata); err != nil {
			return err
		}

		if err := delegationMetadata.Validate(); err != nil {
			return err
		}

		if delegationMetadata.Delegations == nil {
			continue
		}

		for keyID, key := range delegationMetadata.Delegations.Keys {
			delegationKeys[keyID] = key
		}

		delegationsQueue = append(delegationsQueue, delegationMetadata.Delegations.Roles...)
	}

	if len(delegationEnvelopes) != 0 {
		return ErrDanglingDelegationMetadata
	}

	return nil
}

// Commit verifies and writes the State to the policy namespace. It also creates
// an RSL entry recording the new tip of the policy namespace.
func (s *State) Commit(ctx context.Context, repo *git.Repository, commitMessage string, signCommit bool) error {
	if err := s.Verify(ctx); err != nil {
		return err
	}

	if len(commitMessage) == 0 {
		commitMessage = DefaultCommitMessage
	}

	metadata := map[string]*sslibdsse.Envelope{}
	metadata[RootRoleName] = s.RootEnvelope
	if s.TargetsEnvelope != nil {
		metadata[TargetsRoleName] = s.TargetsEnvelope
	}

	if s.DelegationEnvelopes != nil {
		for k, v := range s.DelegationEnvelopes {
			metadata[k] = v
		}
	}

	metadataEntries := []object.TreeEntry{}
	for name, env := range metadata {
		metadataContents, err := json.Marshal(env)
		if err != nil {
			return err
		}

		blobID, err := gitinterface.WriteBlob(repo, metadataContents)
		if err != nil {
			return err
		}

		metadataEntries = append(metadataEntries, object.TreeEntry{
			Name: fmt.Sprintf("%s.json", name),
			Mode: filemode.Regular,
			Hash: blobID,
		})
	}
	metadataTreeID, err := gitinterface.WriteTree(repo, metadataEntries)
	if err != nil {
		return err
	}

	keysEntries := []object.TreeEntry{}
	for _, key := range s.RootPublicKeys {
		keyContents, err := json.Marshal(key)
		if err != nil {
			return err
		}

		blobID, err := gitinterface.WriteBlob(repo, keyContents)
		if err != nil {
			return err
		}

		keysEntries = append(keysEntries, object.TreeEntry{
			Name: key.KeyID,
			Mode: filemode.Regular,
			Hash: blobID,
		})
	}
	keysTreeID, err := gitinterface.WriteTree(repo, keysEntries)
	if err != nil {
		return err
	}

	policyRootTreeID, err := gitinterface.WriteTree(repo, []object.TreeEntry{
		{
			Name: metadataTreeEntryName,
			Mode: filemode.Dir,
			Hash: metadataTreeID,
		},
		{
			Name: rootPublicKeysTreeEntryName,
			Mode: filemode.Dir,
			Hash: keysTreeID,
		},
	})
	if err != nil {
		return err
	}

	ref, err := repo.Reference(plumbing.ReferenceName(PolicyRef), true)
	if err != nil {
		return err
	}
	originalCommitID := ref.Hash()

	commitID, err := gitinterface.Commit(repo, policyRootTreeID, PolicyRef, commitMessage, signCommit)
	if err != nil {
		return err
	}

	// We must reset to original policy commit if err != nil from here onwards.

	if err := rsl.NewReferenceEntry(PolicyRef, commitID).Commit(repo, signCommit); err != nil {
		return gitinterface.ResetDueToError(err, repo, PolicyRef, originalCommitID)
	}

	return nil
}

// GetRootMetadata returns the deserialized payload of the State's RootEnvelope.
func (s *State) GetRootMetadata() (*tuf.RootMetadata, error) {
	payloadBytes, err := s.RootEnvelope.DecodeB64Payload()
	if err != nil {
		return nil, err
	}

	rootMetadata := &tuf.RootMetadata{}
	if err := json.Unmarshal(payloadBytes, rootMetadata); err != nil {
		return nil, err
	}

	return rootMetadata, nil
}

func (s *State) GetTargetsMetadata(roleName string) (*tuf.TargetsMetadata, error) {
	e := s.TargetsEnvelope
	if roleName != TargetsRoleName {
		env, ok := s.DelegationEnvelopes[roleName]
		if !ok {
			return nil, ErrMetadataNotFound
		}
		e = env
	}
	payloadBytes, err := e.DecodeB64Payload()
	if err != nil {
		return nil, err
	}

	targetsMetadata := &tuf.TargetsMetadata{}
	if err := json.Unmarshal(payloadBytes, targetsMetadata); err != nil {
		return nil, err
	}

	return targetsMetadata, nil
}

func (s *State) HasTargetsRole(roleName string) bool {
	if roleName == TargetsRoleName {
		return s.TargetsEnvelope != nil
	}

	_, ok := s.DelegationEnvelopes[roleName]
	return ok
}

func (s *State) findDelegationEntry(roleName string) (tuf.Delegation, error) {
	topLevelTargetsMetadata, err := s.GetTargetsMetadata(TargetsRoleName)
	if err != nil {
		return tuf.Delegation{}, err
	}

	delegationTargetsMetadata := map[string]*tuf.TargetsMetadata{}
	for name, env := range s.DelegationEnvelopes {
		targetsMetadata := &tuf.TargetsMetadata{}

		envBytes, err := env.DecodeB64Payload()
		if err != nil {
			return tuf.Delegation{}, err
		}

		if err := json.Unmarshal(envBytes, targetsMetadata); err != nil {
			return tuf.Delegation{}, err
		}
		delegationTargetsMetadata[name] = targetsMetadata
	}

	delegationsQueue := topLevelTargetsMetadata.Delegations.Roles

	for {
		if len(delegationsQueue) == 0 {
			return tuf.Delegation{}, ErrDelegationNotFound
		}

		delegation := delegationsQueue[0]
		delegationsQueue = delegationsQueue[1:]

		if delegation.Name == roleName {
			return delegation, nil
		}

		if s.HasTargetsRole(delegation.Name) {
			// TODO: clarifying terminating / reachability
			delegationsQueue = append(delegationsQueue, delegationTargetsMetadata[delegation.Name].Delegations.Roles...)
		}
	}
}
