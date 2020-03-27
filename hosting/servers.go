package hosting

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/digitalautonomy/grumble/pkg/logtarget"
	grumbleServer "github.com/digitalautonomy/grumble/server"
)

// TODO[OB] - What's the difference between Shutdown and Cleanup?

// Servers serves
type Servers interface {
	CreateServer(port string, password string) (Server, error)
	DestroyServer(Server) error
	Shutdown() error
	GetDataDir() string
	Cleanup()
}

// MeetingData is a representation of the data used to create a Mumble url
// More information at https://wiki.mumble.info/wiki/Mumble_URL
type MeetingData struct {
	MeetingID string
	Port      int
	Password  string
	Username  string
}

// Create creates
func Create() (Servers, error) {
	s := &servers{}
	e := s.create()
	return s, e
}

type servers struct {
	dataDir string
	started bool
	nextID  int
	servers map[int64]*grumbleServer.Server
	log     *log.Logger
}

// GenerateURL is a helper function for creating Mumble valid URLs
func (d *MeetingData) GenerateURL() string {
	u := url.URL{
		Scheme: "mumble",
		User:   url.UserPassword(d.Username, d.Password),
		Host:   fmt.Sprintf("%s:%d", d.MeetingID, d.Port),
	}

	return u.String()
}

func (s *servers) initializeSharedObjects() {
	s.servers = make(map[int64]*grumbleServer.Server)
	grumbleServer.SetServers(s.servers)
}

func (s *servers) initializeDataDirectory() error {
	var e error
	s.dataDir, e = ioutil.TempDir("", "wahay")
	if e != nil {
		return e
	}

	grumbleServer.Args.DataDir = s.dataDir

	_ = os.MkdirAll(filepath.Join(s.dataDir, "servers"), 0700)

	return nil
}

func (s *servers) initializeLogging() error {
	logDir := path.Join(s.dataDir, "grumble.log")
	grumbleServer.Args.LogPath = logDir

	err := logtarget.Target.OpenFile(logDir)
	if err != nil {
		return err
	}

	l := log.New()
	l.SetOutput(&logtarget.Target)
	s.log = l
	s.log.Info("Grumble")
	s.log.Infof("Using data directory: %s", s.dataDir)

	return nil
}

func (s *servers) initializeCertificates() error {
	s.log.Debug("Generating 4096-bit RSA keypair for self-signed certificate...")

	certFn := filepath.Join(s.dataDir, "cert.pem")
	keyFn := filepath.Join(s.dataDir, "key.pem")
	err := grumbleServer.GenerateSelfSignedCert(certFn, keyFn)
	if err != nil {
		return err
	}

	s.log.Debugf("Certificate output to %v", certFn)
	s.log.Debugf("Private key output to %v", keyFn)
	return nil
}

func callAll(fs ...func() error) error {
	for _, f := range fs {
		if e := f(); e != nil {
			return e
		}
	}
	return nil
}

// create will initialize all grumble things
// because the grumble server package uses global
// state it is NOT advisable to call this function
// more than once in a program
func (s *servers) create() error {
	s.initializeSharedObjects()

	return callAll(
		s.initializeDataDirectory,
		s.initializeLogging,
		s.initializeCertificates,
	)
}

func (s *servers) startListener() {
	if !s.started {
		go grumbleServer.SignalHandler()
		s.started = true
	}
}

func (s *servers) CreateServer(port string, password string) (Server, error) {
	s.nextID++

	serv, err := grumbleServer.NewServer(int64(s.nextID))
	if err != nil {
		return nil, err
	}

	s.servers[serv.Id] = serv
	// We should translate this but the i18n package is not available from here
	serv.Set("WelcomeText", "Welcome to this server running <b>Wahay</b>.")
	serv.Set("NoWebServer", "true")
	serv.Set("Address", "127.0.0.1")
	serv.Set("Port", port)
	if len(password) > 0 {
		serv.SetServerPassword(password)
	}

	err = os.Mkdir(filepath.Join(s.dataDir, "servers", fmt.Sprintf("%v", serv.Id)), 0750)
	if err != nil {
		return nil, err
	}

	return &server{s, serv}, nil
}

func (s *servers) DestroyServer(Server) error {
	// For now, this function will do nothing. We will still call it,
	// in case we need it in the server
	return nil
}

func (s *servers) Shutdown() error {
	return os.RemoveAll(s.dataDir)
}

func (s *servers) GetDataDir() string {
	return s.dataDir
}

func (s *servers) Cleanup() {
	err := os.RemoveAll(s.dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error cleaning up temporaries: "+err.Error())
	}
}
