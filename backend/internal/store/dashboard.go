package store

import (
	"context"
	"revenue-recovery/backend/internal/reporting"
)

func (p *Postgres) DashboardOperational(ctx context.Context) (reporting.Operational, error) {
	value := reporting.Operational{Mode: "OPERATIONAL / LIVE TEST DATA", Cases: []map[string]any{}, RootCauses: []reporting.RootCause{}, ActionSelections: []reporting.ActionSelection{}, RecoveryTimeline: []reporting.RecoveryPoint{}}
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
	if err = rows.Err(); err != nil {
		return value, err
	}
	rootRows, err := p.pool.Query(ctx, `SELECT COALESCE(NULLIF(failure_or_leak_context->>'failure_category',''),'UNCLASSIFIED'),COUNT(*),COALESCE(SUM(amount_at_risk_minor),0) FROM recovery_cases GROUP BY 1 ORDER BY 3 DESC,1`)
	if err != nil {
		return value, err
	}
	for rootRows.Next() {
		var item reporting.RootCause
		if err = rootRows.Scan(&item.Cause, &item.Cases, &item.AmountAtRiskMinor); err != nil {
			rootRows.Close()
			return value, err
		}
		value.RootCauses = append(value.RootCauses, item)
	}
	if err = rootRows.Err(); err != nil {
		rootRows.Close()
		return value, err
	}
	rootRows.Close()
	actionRows, err := p.pool.Query(ctx, `WITH latest AS (SELECT DISTINCT ON(case_id) case_id,selected_action FROM recovery_decisions ORDER BY case_id,created_at DESC,id DESC) SELECT selected_action,COUNT(*) FROM latest GROUP BY selected_action ORDER BY COUNT(*) DESC,selected_action`)
	if err != nil {
		return value, err
	}
	for actionRows.Next() {
		var item reporting.ActionSelection
		if err = actionRows.Scan(&item.Action, &item.Cases); err != nil {
			actionRows.Close()
			return value, err
		}
		value.ActionSelections = append(value.ActionSelections, item)
	}
	if err = actionRows.Err(); err != nil {
		actionRows.Close()
		return value, err
	}
	actionRows.Close()
	timelineRows, err := p.pool.Query(ctx, `SELECT to_char(date_trunc('day',observed_at),'YYYY-MM-DD'),COALESCE(SUM(recovered_amount_minor),0) FROM recovery_attributions GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return value, err
	}
	var cumulative int64
	for timelineRows.Next() {
		var item reporting.RecoveryPoint
		if err = timelineRows.Scan(&item.Day, &item.RecoveredMinor); err != nil {
			timelineRows.Close()
			return value, err
		}
		cumulative += item.RecoveredMinor
		item.CumulativeRecoveredMinor = cumulative
		value.RecoveryTimeline = append(value.RecoveryTimeline, item)
	}
	if err = timelineRows.Err(); err != nil {
		timelineRows.Close()
		return value, err
	}
	timelineRows.Close()
	return value, nil
}
