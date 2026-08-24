package incident

import "time"

type Classifier struct{}

func NewClassifier() *Classifier {
	return &Classifier{}
}

func (c *Classifier) LocalComplete(incident *Incident, requiresPeer bool, at time.Time) Snapshot {
	return incident.update(func(current *Snapshot) {
		if current.State == StateCableCut || current.State == StateStationFault || current.State == StateResolved {
			return
		}
		current.LocalComplete = true
		current.UpdatedAt = at.UTC()

		if current.PeerComplete {
			current.State = StateCableCut
			current.Reason = "paired landing evidence identifies wet span fault"
		} else if requiresPeer {
			current.State = StateWaitingPeer
			current.Reason = "local diagnostics complete; awaiting peer wet span evidence"
		} else {
			current.State = StateStationFault
			current.Reason = "local diagnostics completed without peer wet span evidence"
		}
	})
}

func (c *Classifier) PeerEvidence(incident *Incident, at time.Time) Snapshot {
	return incident.update(func(current *Snapshot) {
		if current.State == StateCableCut || current.State == StateStationFault || current.State == StateResolved {
			return
		}
		current.PeerComplete = true
		current.UpdatedAt = at.UTC()
		if current.LocalComplete {
			current.State = StateCableCut
			current.Reason = "paired landing evidence identifies wet span fault"
		}
	})
}

func (c *Classifier) Expire(incident *Incident, at time.Time) Snapshot {
	return incident.update(func(current *Snapshot) {
		if at.Before(current.Deadline) || current.State != StateWaitingPeer {
			return
		}
		current.State = StateStationFault
		current.Reason = "peer evidence window expired"
		current.UpdatedAt = at.UTC()
	})
}
