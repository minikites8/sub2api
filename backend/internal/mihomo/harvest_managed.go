package mihomo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

// Managed directed harvests lease an exclusive collection selector/listener.
// They never change the business/round-robin selector. UseOnce reservations are
// persisted before traffic and remain retired after errors or cancellation.
func (s *DirectedSidecar) managedDirectory() ([]HarvestNode, error) {
	m := s.managed
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || !m.state.Running {
		return nil, errors.New("managed Mihomo is not running")
	}
	nodes := make([]HarvestNode, 0, len(m.saved.Nodes))
	for _, entry := range m.saved.Nodes {
		id, _ := entry["name"].(string)
		if id == "" || m.saved.Disabled[id] != "" || !countryAllowed(m.saved, id) {
			continue
		}
		name := m.saved.NodeNames[id]
		if name == "" {
			name = id
		}
		nodes = append(nodes, HarvestNode{ID: id, Name: sanitizeNodeDisplayName(name), Provider: "managed", RuntimeID: harvestDigest(entry)})
	}
	if len(nodes) == 0 {
		return nil, errors.New("no eligible proxy nodes")
	}
	return nodes, nil
}

func (s *DirectedSidecar) acquireManaged(ctx context.Context, node HarvestNode) (func(), error) {
	m := s.managed
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	unlock, err := directedLocks.Acquire(wait, fmt.Sprintf("managed-sidecar:%p", s))
	if err != nil {
		return nil, err
	}
	lanes, ports, closeChannels, err := m.reserveCollectionChannels(wait, 1)
	if err != nil {
		unlock()
		return nil, err
	}
	var once sync.Once
	release := func() { once.Do(func() { _ = closeChannels(); unlock() }) }
	s.managedLane = lanes[0]
	s.ProxyURL = fmt.Sprintf("http://127.0.0.1:%d", ports[0])
	nodes, err := s.managedDirectory()
	found := false
	for _, current := range nodes {
		if current.ID == node.ID && current.RuntimeID == node.RuntimeID {
			found = true
			break
		}
	}
	if err != nil || !found {
		release()
		return nil, errors.New("harvest node is no longer eligible")
	}
	m.mu.Lock()
	next := m.saved
	// Directory lookup and retirement form one reservation. Another channel
	// may have retired this candidate while this caller was selecting it.
	if next.UseOnce && next.Disabled[node.ID] != "" {
		m.mu.Unlock()
		release()
		return nil, errors.New("harvest node is already reserved")
	}
	if next.UseOnce {
		next.Disabled = copyNodeStates(next.Disabled)
		next.Disabled[node.ID] = "used"
		data, _ := json.Marshal(next)
		err = atomicWrite(filepath.Join(m.dir, "settings.json"), data, 0600)
		if err == nil {
			m.saved = next
		}
	}
	m.mu.Unlock()
	if err != nil {
		release()
		return nil, errors.New("cannot reserve harvest node")
	}
	payload, _ := json.Marshal(map[string]string{"name": node.ID})
	if err = m.control(ctx, http.MethodPut, "/proxies/"+collectionGroup(s.managedLane), next.Secret, payload); err == nil {
		err = s.confirmManaged(ctx, node)
	}
	if err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (s *DirectedSidecar) confirmManaged(ctx context.Context, node HarvestNode) error {
	m := s.managed
	m.mu.Lock()
	valid := !m.closed && m.state.Running && countryAllowed(m.saved, node.ID) &&
		(m.saved.Disabled[node.ID] == "" || m.saved.Disabled[node.ID] == "used")
	secret := m.saved.Secret
	controller := m.controllerURL
	found := false
	for _, entry := range m.saved.Nodes {
		if entry["name"] == node.ID && harvestDigest(entry) == node.RuntimeID {
			found = true
			break
		}
	}
	m.mu.Unlock()
	if !valid || !found {
		return errors.New("harvest node identity changed")
	}
	if controller == "" {
		controller = "http://127.0.0.1:9098"
	}
	client := &DirectedSidecar{Controller: controller, Secret: secret}
	var group directedProxy
	if err := client.control(ctx, http.MethodGet, "/proxies/"+collectionGroup(s.managedLane), nil, &group); err != nil {
		return err
	}
	if group.Type != "Selector" || group.Now != node.ID {
		return errors.New("harvest selection changed")
	}
	return nil
}
