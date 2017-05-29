package krd

import (
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"

	"github.com/kryptco/kr"
	"github.com/op/go-logging"
)

type ControlServer struct {
	enclaveClient EnclaveClientI
	log           *logging.Logger
}

func NewControlServer(log *logging.Logger, notifier *kr.Notifier) (cs *ControlServer, err error) {
	krdir, err := kr.KrDir()
	if err != nil {
		return
	}
	cs = &ControlServer{UnpairedEnclaveClient(
		kr.AWSTransport{},
		kr.FilePersister{
			PairingDir: krdir,
			SSHDir:     filepath.Join(kr.UnsudoedHomeDir(), ".ssh"),
		},
		nil,
		log,
		notifier,
	),
		log,
	}
	return
}

func (cs *ControlServer) HandleControlHTTP(listener net.Listener) (err error) {
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/pair", cs.handlePair)
	httpMux.HandleFunc("/enclave", cs.handleEnclave)
	httpMux.HandleFunc("/ping", cs.handlePing)
	err = http.Serve(listener, httpMux)
	return
}

func (cs *ControlServer) Start() (err error) {
	return cs.enclaveClient.Start()
}

func (cs *ControlServer) Stop() (err error) {
	return cs.enclaveClient.Stop()
}

func (cs *ControlServer) EnclaveClient() EnclaveClientI {
	return cs.enclaveClient
}

func (cs *ControlServer) handlePair(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cs.handleGetPair(w, r)
		return
	case http.MethodPut:
		cs.handlePutPair(w, r)
		return
	case http.MethodDelete:
		cs.handleDeletePair(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func (cs *ControlServer) handleDeletePair(w http.ResponseWriter, r *http.Request) {
	cs.enclaveClient.Unpair()
	w.WriteHeader(http.StatusOK)
	return
}

//	check if pairing completed
func (cs *ControlServer) handleGetPair(w http.ResponseWriter, r *http.Request) {
	meResponse, err := cs.enclaveClient.RequestMe(true)
	if err == nil && meResponse != nil {
		err = json.NewEncoder(w).Encode(meResponse.Me)
		if err != nil {
			cs.log.Error(err)
			return
		}
	} else {
		w.WriteHeader(http.StatusNotFound)
		if err != nil {
			cs.log.Error(err)
		}
		return
	}
}

//	initiate new pairing (clearing any existing)
func (cs *ControlServer) handlePutPair(w http.ResponseWriter, r *http.Request) {
	pairingSecret, err := cs.enclaveClient.Pair()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		cs.log.Error(err)
		return
	}
	err = json.NewEncoder(w).Encode(pairingSecret)
	if err != nil {
		cs.log.Error(err)
		return
	}
}

//	route request to enclave
func (cs *ControlServer) handleEnclave(w http.ResponseWriter, r *http.Request) {
	var enclaveRequest kr.Request
	err := json.NewDecoder(r.Body).Decode(&enclaveRequest)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if enclaveRequest.MeRequest != nil {
		cs.handleEnclaveMe(w, enclaveRequest)
		return
	}

	if enclaveRequest.SignRequest != nil {
		cs.handleEnclaveSign(w, enclaveRequest)
		return
	}

	if enclaveRequest.GitSignRequest != nil {
		cs.handleEnclaveGitSign(w, enclaveRequest)
		return
	}

	cs.enclaveClient.RequestNoOp()

	w.WriteHeader(http.StatusOK)
}

func (cs *ControlServer) handleEnclaveMe(w http.ResponseWriter, enclaveRequest kr.Request) {
	var me kr.Profile
	cachedMe := cs.enclaveClient.GetCachedMe()
	if cachedMe != nil {
		me = *cachedMe
	} else {
		meResponse, err := cs.enclaveClient.RequestMe(false)
		if err != nil {
			cs.log.Error("me request error:", err)
			switch err {
			case ErrNotPaired:
				w.WriteHeader(http.StatusNotFound)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		if meResponse != nil {
			me = meResponse.Me
		} else {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}
	response := kr.Response{
		MeResponse: &kr.MeResponse{
			Me: me,
		},
	}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		cs.log.Error(err)
		return
	}
}

func (cs *ControlServer) handleEnclaveSign(w http.ResponseWriter, enclaveRequest kr.Request) {
	signResponse, _, err := cs.enclaveClient.RequestSignature(*enclaveRequest.SignRequest, nil)
	if err != nil {
		cs.log.Error("signature request error:", err)
		switch err {
		case ErrNotPaired:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	if signResponse != nil {
		response := kr.Response{
			RequestID:    enclaveRequest.RequestID,
			SignResponse: signResponse,
		}
		err = json.NewEncoder(w).Encode(response)
		if err != nil {
			cs.log.Error(err)
			return
		}
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}
func (cs *ControlServer) handleEnclaveGitSign(w http.ResponseWriter, enclaveRequest kr.Request) {
	signResponse, _, err := cs.enclaveClient.RequestGitSignature(
		*enclaveRequest.GitSignRequest,
		func() {
			cs.notify(enclaveRequest.NotifyPrefix(), kr.Yellow("Kryptonite ▶ Phone approval required. Respond using the Kryptonite app"))
		})

	if err != nil {
		cs.log.Error("signature request error:", err)
		switch err {
		case ErrNotPaired:
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	if signResponse != nil {
		response := kr.Response{
			RequestID:       enclaveRequest.RequestID,
			GitSignResponse: signResponse,
		}
		err = json.NewEncoder(w).Encode(response)
		if err != nil {
			cs.log.Error(err)
			return
		}
	} else {
		w.WriteHeader(http.StatusNotFound)
	}

}

func (cs *ControlServer) handlePing(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (cs *ControlServer) notify(prefix, body string) {
	n, err := kr.OpenNotifier(prefix)
	if err != nil {
		cs.log.Error("error writing notification: " + err.Error())
		return
	}
	defer n.Close()
	err = n.Notify(append([]byte(body), '\r', '\n'))
	if err != nil {
		cs.log.Error("error writing notification: " + err.Error())
		return
	}
}
