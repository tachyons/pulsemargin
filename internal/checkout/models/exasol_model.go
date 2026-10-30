package models

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"pulsemargin/internal/checkout"
)

// ExasolMLModel executes machine learning model inference in-database on an Exasol cluster.
type ExasolMLModel struct {
	db        *sql.DB
	modelName string
	fallback  checkout.MLModel
}

// NewExasolMLModel creates an MLModel that runs inference directly inside Exasol.
func NewExasolMLModel(db *sql.DB, modelName string, fallback checkout.MLModel) *ExasolMLModel {
	if modelName == "" {
		modelName = "exasol_ensemble_v2"
	}
	return &ExasolMLModel{
		db:        db,
		modelName: modelName,
		fallback:  fallback,
	}
}

func (m *ExasolMLModel) Name() string {
	return m.modelName
}

// Predict calls the in-database predict_ml_risk UDF on Exasol.
func (m *ExasolMLModel) Predict(ctx context.Context, features checkout.Features) (checkout.Prediction, error) {
	if m.db == nil {
		if m.fallback != nil {
			return m.fallback.Predict(ctx, features)
		}
		return checkout.Prediction{}, fmt.Errorf("exasol database connection is not available")
	}

	query := `
		SELECT predict_ml_risk(
			?,
			?,
			?,
			CAST(? AS BOOLEAN),
			?,
			CAST(? AS DOUBLE PRECISION),
			CAST(? AS BOOLEAN)
		);
	`

	var resultJSON sql.NullString
	err := m.db.QueryRowContext(ctx, query,
		m.modelName,
		features.CustomerReturnRate,
		features.PincodeRTORate,
		strconv.FormatBool(features.HasBracketing),
		features.MaxSizeSkew,
		strconv.FormatFloat(features.CartTotal, 'f', -1, 64),
		strconv.FormatBool(features.IsCOD),
	).Scan(&resultJSON)

	if err != nil || !resultJSON.Valid {
		if m.fallback != nil {
			return m.fallback.Predict(ctx, features)
		}
		return checkout.Prediction{}, fmt.Errorf("exasol in-database ml inference failed: %w", err)
	}

	var pred checkout.Prediction
	if err := json.Unmarshal([]byte(resultJSON.String), &pred); err != nil {
		if m.fallback != nil {
			return m.fallback.Predict(ctx, features)
		}
		return checkout.Prediction{}, fmt.Errorf("decode exasol ml prediction: %w", err)
	}

	return pred, nil
}
