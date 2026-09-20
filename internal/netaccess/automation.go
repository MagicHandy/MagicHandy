package netaccess

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/netutil"
)

// CertificateProvider is the common listener contract for file and managed TLS.
type CertificateProvider interface {
	TLSConfig() *tls.Config
	Status() CertificateStatus
}

// PreparationStatus is a bounded, public snapshot of one explicit setup action.
type PreparationStatus struct {
	State       string             `json:"state"`
	Config      *Config            `json:"config,omitempty"`
	Message     string             `json:"message,omitempty"`
	Certificate *CertificateStatus `json:"certificate,omitempty"`
}

// Automation owns certificate work, never account authority or app listeners.
// Initial public issuance opens a temporary challenge-only socket. Saving the
// network configuration is a separate password-confirmed transaction, after a
// valid certificate exists. Renewal uses the already-authorized TLS listener.
type Automation struct {
	root        string
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	operation   sync.Mutex
	wg          sync.WaitGroup
	closed      bool
	started     bool
	status      PreparationStatus
	active      *managedCertificates
	lastAttempt time.Time
	discovering bool
	discovered  time.Time
	discovery   InternetDiscovery
	issue       func(context.Context, *Policy, *challengeSolver) ([]byte, error)
}

// NewAutomation does not create files, contact services, or open sockets.
func NewAutomation(dataDir string) *Automation {
	ctx, cancel := context.WithCancel(context.Background())
	a := &Automation{root: filepath.Join(dataDir, "https-private"), ctx: ctx, cancel: cancel, status: PreparationStatus{State: "idle"}}
	a.issue = a.issuePublic
	return a
}

// LoadManaged fails closed if setup has not yet produced a valid certificate.
func (a *Automation) LoadManaged(policy *Policy) (CertificateProvider, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	certs, err := a.load(policy)
	if err != nil {
		// A previously prepared certificate can expire while the host is off.
		// Retain its identity for renewal; getCertificate still rejects expired
		// chains, so no browser receives it while renewal is pending.
		certs, err = a.loadExpired(policy)
		if err != nil {
			return nil, err
		}
	}
	a.active = &managedCertificates{Certificates: certs, policy: policy, solver: &challengeSolver{host: policy.Origin.Hostname()}}
	return a.active, nil
}

func (a *Automation) loadExpired(policy *Policy) (*Certificates, error) {
	path := certificatePath(a.root, policy)
	data, err := readPrivate(path)
	if err != nil {
		return nil, errors.New("prepare HTTPS in local setup before enabling remote access")
	}
	pair, err := tls.X509KeyPair(data, data)
	if err != nil {
		return nil, errors.New("stored HTTPS certificate is invalid")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || time.Now().Before(leaf.NotAfter) {
		return nil, errors.New("stored HTTPS certificate is not valid")
	}
	checked, err := validateCertificatePair(pair, policy.Origin.Hostname(), leaf.NotAfter.Add(-time.Second))
	if err != nil {
		return nil, err
	}
	c := policy.Config
	c.TLSCertificate, c.TLSPrivateKey = path, path
	return &Certificates{config: c, host: policy.Origin.Hostname(), pair: checked, checked: time.Now()}, nil
}

func (a *Automation) load(policy *Policy) (*Certificates, error) {
	if policy.Config.CertificateMode == "" {
		return nil, errors.New("choose an automatic certificate mode")
	}
	copyPolicy := *policy
	path := certificatePath(a.root, policy)
	copyPolicy.Config.TLSCertificate, copyPolicy.Config.TLSPrivateKey = path, path
	certs, err := LoadCertificates(&copyPolicy)
	if err != nil {
		return nil, errors.New("prepare a valid HTTPS certificate before saving or restarting; use -network-mode local for recovery")
	}
	return certs, nil
}

// ValidatePrepared only reads the saved chain; it cannot cause issuance.
func (a *Automation) ValidatePrepared(policy *Policy) error { _, err := a.load(policy); return err }

// Prepare starts at most one job and limits repeated public validation attempts.
func (a *Automation) Prepare(policy *Policy) (PreparationStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return a.status, errors.New("certificate service is shutting down")
	}
	if a.status.State == "running" {
		return a.status, errors.New("certificate preparation is already running")
	}
	config := policy.Config
	if certs, err := a.load(policy); err == nil && !managedRenewalDue(certs.Status(), time.Now()) {
		status := certs.Status()
		a.status = PreparationStatus{State: "ready", Config: &config, Certificate: &status}
		return a.status, nil
	}
	if policy.Config.CertificateMode == AutomaticPublic && time.Since(a.lastAttempt) < time.Minute {
		return a.status, errors.New("wait one minute before retrying public certificate validation")
	}
	a.lastAttempt = time.Now()
	a.status = PreparationStatus{State: "running", Config: &config}
	a.wg.Add(1)
	go a.prepare(policy)
	return a.status, nil
}

func (a *Automation) prepare(policy *Policy) {
	defer a.wg.Done()
	a.operation.Lock()
	defer a.operation.Unlock()
	ctx, cancel := context.WithTimeout(a.ctx, 4*time.Minute)
	defer cancel()
	solver := &challengeSolver{host: policy.Origin.Hostname()}
	err := preparePrivateDirectory(a.root)
	if err == nil && policy.Config.CertificateMode == AutomaticPublic {
		var closeListener func()
		closeListener, err = startChallengeListener(ctx, policy.Config.ListenAddress, solver)
		if err == nil {
			defer closeListener()
		}
	}
	if err == nil {
		err = a.obtain(ctx, policy, solver)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.status.State, a.status.Message = "failed", err.Error()
		return
	}
	certs, err := a.load(policy)
	if err != nil {
		a.status.State, a.status.Message = "failed", err.Error()
		return
	}
	status := certs.Status()
	a.status.State, a.status.Certificate = "ready", &status
}

func (a *Automation) obtain(ctx context.Context, policy *Policy, solver *challengeSolver) error {
	var data []byte
	var err error
	if policy.Config.CertificateMode == LocalCA {
		data, err = issueLocalCertificate(a.root, policy.Origin.Hostname())
	} else {
		data, err = a.issue(ctx, policy, solver)
	}
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return errors.New("certificate preparation was canceled")
	}
	if _, err := parseCertificate(data, data, policy.Origin.Hostname(), time.Now()); err != nil {
		return err
	}
	return writePrivate(certificatePath(a.root, policy), data)
}

// Snapshot never includes keys, certificate paths, or provider error bodies.
func (a *Automation) Snapshot() PreparationStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

// StartRenewal is called once the app owns the selected TLS listener. Issuance
// is scheduled independently of browser requests, preventing handshake abuse.
func (a *Automation) StartRenewal() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.started || a.active == nil {
		return
	}
	a.started = true
	a.wg.Add(1)
	go a.renewLoop(a.active)
}

func (a *Automation) renewLoop(active *managedCertificates) {
	defer a.wg.Done()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	retry := time.Minute
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-timer.C:
		}
		if managedRenewalDue(active.Status(), time.Now()) {
			a.operation.Lock()
			ctx, cancel := context.WithTimeout(a.ctx, 4*time.Minute)
			err := preparePrivateDirectory(a.root)
			if err == nil {
				err = a.obtain(ctx, active.policy, active.solver)
			}
			cancel()
			a.operation.Unlock()
			active.mu.Lock()
			active.reloadOK = err == nil
			if err == nil {
				pair, loadErr := loadCertificate(active.config, active.host, time.Now())
				if loadErr == nil {
					active.pair, active.checked = pair, time.Now()
				} else {
					active.reloadOK = false
				}
			}
			active.mu.Unlock()
			active.renewalFailed.Store(err != nil)
			if err != nil {
				timer.Reset(retry)
				retry = min(retry*2, time.Hour)
				continue
			}
		}
		retry = time.Minute
		timer.Reset(15 * time.Minute)
	}
}

func managedRenewalDue(status CertificateStatus, now time.Time) bool {
	return !now.Before(status.NotBefore.Add(status.NotAfter.Sub(status.NotBefore) / 2))
}

// Close cancels network operations and joins every job before storage teardown.
func (a *Automation) Close() {
	a.mu.Lock()
	a.closed = true
	a.cancel()
	a.mu.Unlock()
	a.wg.Wait()
}

func startChallengeListener(ctx context.Context, address string, solver *challengeSolver) (func(), error) {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		return nil, errors.New("cannot open the selected certificate port; check the local IP, port permissions, or another app using this port")
	}
	server := &http.Server{Handler: http.NotFoundHandler(), TLSConfig: solver.config(nil),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 1024}
	server.ErrorLog = log.New(io.Discard, "", 0)
	done := make(chan struct{})
	go func() { defer close(done); _ = server.ServeTLS(netutil.LimitListener(listener, 16), "", "") }()
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	return func() { stop(); _ = server.Close(); <-done }, nil
}

type managedCertificates struct {
	*Certificates
	policy        *Policy
	solver        *challengeSolver
	renewalFailed atomic.Bool
}

func (c *managedCertificates) TLSConfig() *tls.Config { return c.solver.config(c.getCertificate) }
func (c *managedCertificates) Status() CertificateStatus {
	status := c.Certificates.Status()
	status.Managed = true
	status.ReloadError = status.ReloadError || c.renewalFailed.Load()
	status.RenewalDue = managedRenewalDue(status, time.Now())
	return status
}
