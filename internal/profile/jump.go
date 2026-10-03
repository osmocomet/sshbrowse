package profile

import "fmt"

const MaxJumpHops = 8

// JumpChain resolves saved hops in connection order. Raw ProxyJump routes stay
// with OpenSSH and terminate the saved chain.
func JumpChain(connection Connection, saved []Connection) ([]Connection, error) {
	byID := make(map[string]Connection, len(saved))
	for _, item := range saved {
		byID[item.ID] = item
	}
	return jumpChain(connection, byID)
}

func jumpChain(connection Connection, saved map[string]Connection) ([]Connection, error) {
	seen := make(map[string]bool)
	if connection.ID != "" {
		seen[connection.ID] = true
	}
	var hops []Connection
	for connection.JumpConnectionID != "" {
		if connection.JumpHost != "" {
			return nil, fmt.Errorf("choose either a saved jump connection or raw ProxyJump")
		}
		id := connection.JumpConnectionID
		if seen[id] {
			return nil, fmt.Errorf("saved jump connections contain a cycle")
		}
		if len(hops) >= MaxJumpHops {
			return nil, fmt.Errorf("saved jump chain exceeds %d hops", MaxJumpHops)
		}
		hop, ok := saved[id]
		if !ok {
			return nil, fmt.Errorf("saved jump connection %q no longer exists; choose another jump host", id)
		}
		if hop.Host == "" {
			return nil, fmt.Errorf("saved jump connection has no host")
		}
		seen[id] = true
		hops = append(hops, hop)
		connection = hop
	}
	return hops, nil
}

// Check the edited route and its dependents against the proposed snapshot.
// Unrelated broken references (for example a deleted hop) do not block edits.
func validateJumpUpdate(id string, connections []Connection) error {
	byID := make(map[string]Connection, len(connections))
	dependents := make(map[string][]string)
	for _, connection := range connections {
		byID[connection.ID] = connection
		dependents[connection.JumpConnectionID] = append(dependents[connection.JumpConnectionID], connection.ID)
	}
	pending := []string{id}
	checked := make(map[string]bool)
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if checked[current] {
			continue
		}
		checked[current] = true
		if _, err := jumpChain(byID[current], byID); err != nil {
			return fmt.Errorf("connection %q: %w", byID[current].Name, err)
		}
		pending = append(pending, dependents[current]...)
	}
	return nil
}
