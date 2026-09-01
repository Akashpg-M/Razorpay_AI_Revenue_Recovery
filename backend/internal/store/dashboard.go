package store

import (
	"context"
	"revenue-recovery/backend/internal/reporting"
)

func (p *Postgres) DashboardOperational(ctx context.Context) (reporting.Operational, error) {
	value := reporting.Operational{Mode: "OPERATIONAL / LIVE TEST DATA", Cases: []map[string]any{}}
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount_at_risk_minor),0),COALESCE(SUM(recovered_amount_minor),0),COUNT(*) FILTER(WHERE current_state NOT IN('RECOVERED','ESCALATED','EXHAUSTED','STOPPED')) FROM recovery_cases`).Scan(&value.RevenueAtRiskMinor, &value.RecoveredMinor, &value.ActiveCases)
	if err != nil {
		return value, err
	}
	err = p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(recovered_amount_minor) FILTER(WHERE category='NATURAL_RECOVERY'),0),COALESCE(SUM(recovered_amount_minor) FILTER(WHERE category IN('DIRECT_ACTION_ATTRIBUTED','RETRY_ATTRIBUTED','PTP_ATTRIBUTED')),0) FROM recovery_attributions`).Scan(&value.NaturalRecoveredMinor, &value.AgentAttributedMinor)
	if err != nil {
		return value, err
	}
	rows, err := p.pool.Query(ctx, `SELECT jsonb_build_object('case_id',id,'merchant_id',merchant_id,'leak_type',leak_type,'amount_at_risk_minor',amount_at_risk_minor,'current_state',current_state,'created_at',created_at) FROM recovery_cases ORDER BY updated_at DESC LIMIT 12`)
	if err != nil {
		return value, err
	}
	defer rows.Close()
	for rows.Next() {
		var item map[string]any
		if err = rows.Scan(&item); err != nil {
			return value, err
		}
		value.Cases = append(value.Cases, item)
	}
	return value, rows.Err()
}
