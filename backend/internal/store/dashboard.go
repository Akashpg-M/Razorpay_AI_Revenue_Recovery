package store

import (
	"context"
	"revenue-recovery/backend/internal/reporting"
)

func (p *Postgres) DashboardOperational(ctx context.Context) (reporting.Operational, error) {
	value := reporting.Operational{Mode: "OPERATIONAL / LIVE TEST DATA", Cases: []map[string]any{}}
	err := p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(amount_at_risk_minor),0),COALESCE(SUM(recovered_amount_minor),0),
		COUNT(*) FILTER(WHERE current_state NOT IN('RECOVERED','ESCALATED','EXHAUSTED','STOPPED')),
		COUNT(*) FILTER(WHERE current_state='ESCALATED'),
		(SELECT COUNT(*) FROM scheduled_actions WHERE status IN('PENDING','CLAIMED','EXECUTING','RETRY_PENDING','OBSERVATION_PENDING','OBSERVATION_CLAIMED')),
		CASE WHEN COUNT(*)=0 THEN 0 ELSE COUNT(*) FILTER(WHERE current_state='RECOVERED')::float8/COUNT(*)::float8 END
		FROM recovery_cases`).Scan(&value.RevenueAtRiskMinor, &value.RecoveredMinor, &value.ActiveCases, &value.CasesAwaitingReview, &value.ActionsScheduled, &value.RecoveryRate)
	if err != nil {
		return value, err
	}
	err = p.pool.QueryRow(ctx, `SELECT COALESCE(SUM(recovered_amount_minor) FILTER(WHERE category='NATURAL_RECOVERY'),0),COALESCE(SUM(recovered_amount_minor) FILTER(WHERE category IN('DIRECT_ACTION_ATTRIBUTED','RETRY_ATTRIBUTED','PTP_ATTRIBUTED')),0) FROM recovery_attributions`).Scan(&value.NaturalRecoveredMinor, &value.AgentAttributedMinor)
	if err != nil {
		return value, err
	}
	rows, err := p.pool.Query(ctx, `SELECT jsonb_build_object('case_id',c.id,'merchant_id',c.merchant_id,'leak_type',c.leak_type,'amount_at_risk_minor',c.amount_at_risk_minor,'current_state',c.current_state,'created_at',c.created_at,'updated_at',c.updated_at,'recovery_deadline',c.recovery_deadline,'recovered_amount_minor',c.recovered_amount_minor,'attribution_status',c.attribution_status,'last_action',COALESCE((SELECT action_type FROM recovery_actions a WHERE a.case_id=c.id ORDER BY a.created_at DESC LIMIT 1),''),'expected_nerv_minor',COALESCE((SELECT selected_nerv_minor FROM recovery_decisions d WHERE d.case_id=c.id ORDER BY d.created_at DESC LIMIT 1),0),'policy_state',COALESCE((SELECT result FROM policy_evaluations pe WHERE pe.case_id=c.id ORDER BY pe.created_at DESC LIMIT 1),'NOT_EVALUATED')) FROM recovery_cases c ORDER BY c.updated_at DESC LIMIT 50`)
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
