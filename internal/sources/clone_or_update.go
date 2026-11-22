package source

import (
	"context"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	gittransport "github.com/go-git/go-git/v5/plumbing/transport"
	httpAuth "github.com/go-git/go-git/v5/plumbing/transport/http"
	sshAuth "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	configsv1alpha1 "github.com/joe-bresee/gitsync/api/v1alpha1"
)

func cloneOrUpdate(
	ctx context.Context,
	c client.Client,
	repoURL, revision, branch, authMethod string,
	authSecretRef *configsv1alpha1.ObjectRef,
) (string, string, string, error) {

	logger := log.FromContext(ctx)

	cachePath, err := os.MkdirTemp("", "config-sync-*")
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create temporary checkout: %w", err)
	}
	keepCheckout := false
	defer func() {
		if !keepCheckout {
			_ = os.RemoveAll(cachePath)
		}
	}()
	logger.Info("preparing isolated repository checkout", "path", cachePath)

	// Build auth if needed
	authMethodObj, err := buildAuth(ctx, c, authMethod, authSecretRef)
	if err != nil {
		return "", "", "", err
	}

	logger.Info("cloning repository", "url", repoURL, "branch", branch)
	cloneOpts := &git.CloneOptions{
		URL:  repoURL,
		Tags: git.AllTags,
	}
	if authMethodObj != nil {
		cloneOpts.Auth = authMethodObj
	}
	if branch != "" {
		cloneOpts.SingleBranch = true
		cloneOpts.ReferenceName = plumbing.NewBranchReferenceName(branch)
	}

	repo, err := git.PlainClone(cachePath, false, cloneOpts)
	if err != nil {
		return "", "", "", fmt.Errorf("clone failed: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get worktree: %w", err)
	}

	if revision != "" {
		logger.Info("checking out revision", "revision", revision)
		hash, err := resolveRevision(repo, revision)
		if err != nil {
			return "", "", "", fmt.Errorf("resolve revision %q failed: %w", revision, err)
		}
		err = w.Checkout(&git.CheckoutOptions{
			Hash:  hash,
			Force: true,
		})
		if err != nil {
			return "", "", "", fmt.Errorf("checkout revision failed: %w", err)
		}
	} else if branch != "" {
		logger.Info("checking out branch", "branch", branch)
		// Prefer checking out the remote origin/<branch> reference (updated by Fetch),
		// falling back to local branch checkout if the remote ref is not present.
		remoteRef, refErr := repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
		if refErr == nil {
			err = w.Checkout(&git.CheckoutOptions{
				Hash:  remoteRef.Hash(),
				Force: true,
			})
		} else {
			err = w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewBranchReferenceName(branch),
			})
		}
		if err != nil {
			return "", "", "", fmt.Errorf("checkout branch failed: %w", err)
		}
	}

	head, err := repo.Head()
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read HEAD: %w", err)
	}

	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read commit object: %w", err)
	}

	logger.Info("repository synced",
		"repo", repoURL,
		"commit", head.Hash().String(),
		"message", commit.Message,
	)

	keepCheckout = true
	return head.Hash().String(), cachePath, commit.Message, nil
}

func resolveRevision(repo *git.Repository, revision string) (plumbing.Hash, error) {
	hash, err := repo.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("revision %q is not a branch, tag, or commit SHA", revision)
	}
	return *hash, nil
}

func firstSecretKey(data map[string][]byte, keys []string) []byte {
	for _, k := range keys {
		if v, ok := data[k]; ok && len(v) > 0 {
			return v
		}
	}
	return nil
}

func firstSecretString(data map[string][]byte, keys []string) string {
	b := firstSecretKey(data, keys)
	if b == nil {
		return ""
	}
	return string(b)
}

func buildAuth(
	ctx context.Context,
	c client.Client,
	authMethod string,
	authSecretRef *configsv1alpha1.ObjectRef,
) (gittransport.AuthMethod, error) {

	logger := log.FromContext(ctx)

	if strings.EqualFold(authMethod, "none") || authMethod == "" {
		return nil, nil
	}

	if authSecretRef == nil {
		return nil, fmt.Errorf("authSecretRef required for authMethod=%s", authMethod)
	}

	var secret corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: authSecretRef.Namespace,
		Name:      authSecretRef.Name,
	}, &secret); err != nil {
		return nil, fmt.Errorf("failed to read Secret %s/%s: %w",
			authSecretRef.Namespace, authSecretRef.Name, err)
	}

	switch strings.ToLower(authMethod) {
	case "https":
		username := firstSecretString(secret.Data, []string{"username", "user"})
		password := firstSecretString(secret.Data, []string{"password", "pass", "token"})

		if username == "" || password == "" {
			return nil, fmt.Errorf("secret %s/%s missing username/password for https auth",
				authSecretRef.Namespace, authSecretRef.Name)
		}

		logger.Info("using HTTPS basic auth: found username and password", "namespace", authSecretRef.Namespace, "name", authSecretRef.Name)
		return &httpAuth.BasicAuth{
			Username: username,
			Password: password,
		}, nil

	case "ssh":
		key := firstSecretKey(secret.Data, []string{"sshKey", "id_rsa", "ssh-privatekey", "private_key"})
		if key == nil {
			return nil, fmt.Errorf("secret %s/%s missing private key for ssh auth",
				authSecretRef.Namespace, authSecretRef.Name)
		}

		logger.Info("using SSH key auth: found private key", "namespace", authSecretRef.Namespace, "name", authSecretRef.Name)

		signer, err := sshAuth.NewPublicKeys("git", key, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load ssh private key: %w", err)
		}

		knownHosts := firstSecretKey(secret.Data, []string{"known_hosts", "known-hosts"})
		if knownHosts == nil {
			return nil, fmt.Errorf("secret %s/%s missing known_hosts for ssh host verification",
				authSecretRef.Namespace, authSecretRef.Name)
		}
		signer.HostKeyCallback, err = knownHostsCallback(knownHosts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse known_hosts for Secret %s/%s: %w",
				authSecretRef.Namespace, authSecretRef.Name, err)
		}

		return signer, nil

	default:
		return nil, fmt.Errorf("unsupported auth method, or something went wrong: %s", authMethod)
	}
}

func knownHostsCallback(data []byte) (ssh.HostKeyCallback, error) {
	file, err := os.CreateTemp("", "config-sync-known-hosts-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary known_hosts file: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)

	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("restrict temporary known_hosts file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return nil, fmt.Errorf("write temporary known_hosts file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close temporary known_hosts file: %w", err)
	}

	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, err
	}
	return callback, nil
}
