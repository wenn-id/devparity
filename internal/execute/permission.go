package execute

import "errors"

type Grant struct {
	host      bool
	container bool
}

func NewHostGrant(trusted bool) (Grant, error) {
	if !trusted {
		return Grant{}, errors.New("host execution requires repository trust")
	}
	return Grant{host: true}, nil
}
