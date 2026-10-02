package service

import (
	"context"
	"errors"

	"github.com/elug3/dupli1/auth/pkg/domain"
	"github.com/elug3/dupli1/auth/pkg/ports"
	"github.com/elug3/dupli1/shared/pkg/reportperiod"
)

// ErrReportsUnavailable is returned when no RegistrationStats store is wired
// (a deploy without Postgres).
var ErrReportsUnavailable = errors.New("registration reports unavailable")

// WithRegistrationStats sets the store the registrations report reads.
func WithRegistrationStats(stats ports.RegistrationStats) ServiceOption {
	return func(s *Service) { s.registrationStats = stats }
}

// RegistrationReport counts customer sign-ups per week or month; see
// reportperiod.NewRange for how granularity, from and to are read. Managers
// and service accounts are not customers and are not counted.
func (s *Service) RegistrationReport(ctx context.Context, granularity, from, to string) (domain.RegistrationReport, error) {
	r, err := reportperiod.NewRange(granularity, from, to, s.clock())
	if err != nil {
		return domain.RegistrationReport{}, err
	}
	if s.registrationStats == nil {
		return domain.RegistrationReport{}, ErrReportsUnavailable
	}
	times, undated, err := s.registrationStats.RegistrationTimes(ctx, domain.AccountTypeCustomer, r.Start, r.End)
	if err != nil {
		return domain.RegistrationReport{}, err
	}
	return domain.BuildRegistrationReport(r, times, undated), nil
}
