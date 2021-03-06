// Code generated by go-swagger; DO NOT EDIT.

package products

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"
	"net/http"
	"time"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	cr "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
)

// NewGetReplicationExecutionsIDTasksParams creates a new GetReplicationExecutionsIDTasksParams object
// with the default values initialized.
func NewGetReplicationExecutionsIDTasksParams() *GetReplicationExecutionsIDTasksParams {
	var ()
	return &GetReplicationExecutionsIDTasksParams{

		timeout: cr.DefaultTimeout,
	}
}

// NewGetReplicationExecutionsIDTasksParamsWithTimeout creates a new GetReplicationExecutionsIDTasksParams object
// with the default values initialized, and the ability to set a timeout on a request
func NewGetReplicationExecutionsIDTasksParamsWithTimeout(timeout time.Duration) *GetReplicationExecutionsIDTasksParams {
	var ()
	return &GetReplicationExecutionsIDTasksParams{

		timeout: timeout,
	}
}

// NewGetReplicationExecutionsIDTasksParamsWithContext creates a new GetReplicationExecutionsIDTasksParams object
// with the default values initialized, and the ability to set a context for a request
func NewGetReplicationExecutionsIDTasksParamsWithContext(ctx context.Context) *GetReplicationExecutionsIDTasksParams {
	var ()
	return &GetReplicationExecutionsIDTasksParams{

		Context: ctx,
	}
}

// NewGetReplicationExecutionsIDTasksParamsWithHTTPClient creates a new GetReplicationExecutionsIDTasksParams object
// with the default values initialized, and the ability to set a custom HTTPClient for a request
func NewGetReplicationExecutionsIDTasksParamsWithHTTPClient(client *http.Client) *GetReplicationExecutionsIDTasksParams {
	var ()
	return &GetReplicationExecutionsIDTasksParams{
		HTTPClient: client,
	}
}

/*GetReplicationExecutionsIDTasksParams contains all the parameters to send to the API endpoint
for the get replication executions ID tasks operation typically these are written to a http.Request
*/
type GetReplicationExecutionsIDTasksParams struct {

	/*ID
	  The execution ID.

	*/
	ID int64
	/*Page
	  The page number.

	*/
	Page *int32
	/*PageSize
	  The size of per page.

	*/
	PageSize *int32

	timeout    time.Duration
	Context    context.Context
	HTTPClient *http.Client
}

// WithTimeout adds the timeout to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) WithTimeout(timeout time.Duration) *GetReplicationExecutionsIDTasksParams {
	o.SetTimeout(timeout)
	return o
}

// SetTimeout adds the timeout to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) SetTimeout(timeout time.Duration) {
	o.timeout = timeout
}

// WithContext adds the context to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) WithContext(ctx context.Context) *GetReplicationExecutionsIDTasksParams {
	o.SetContext(ctx)
	return o
}

// SetContext adds the context to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) SetContext(ctx context.Context) {
	o.Context = ctx
}

// WithHTTPClient adds the HTTPClient to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) WithHTTPClient(client *http.Client) *GetReplicationExecutionsIDTasksParams {
	o.SetHTTPClient(client)
	return o
}

// SetHTTPClient adds the HTTPClient to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) SetHTTPClient(client *http.Client) {
	o.HTTPClient = client
}

// WithID adds the id to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) WithID(id int64) *GetReplicationExecutionsIDTasksParams {
	o.SetID(id)
	return o
}

// SetID adds the id to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) SetID(id int64) {
	o.ID = id
}

// WithPage adds the page to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) WithPage(page *int32) *GetReplicationExecutionsIDTasksParams {
	o.SetPage(page)
	return o
}

// SetPage adds the page to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) SetPage(page *int32) {
	o.Page = page
}

// WithPageSize adds the pageSize to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) WithPageSize(pageSize *int32) *GetReplicationExecutionsIDTasksParams {
	o.SetPageSize(pageSize)
	return o
}

// SetPageSize adds the pageSize to the get replication executions ID tasks params
func (o *GetReplicationExecutionsIDTasksParams) SetPageSize(pageSize *int32) {
	o.PageSize = pageSize
}

// WriteToRequest writes these params to a swagger request
func (o *GetReplicationExecutionsIDTasksParams) WriteToRequest(r runtime.ClientRequest, reg strfmt.Registry) error {

	if err := r.SetTimeout(o.timeout); err != nil {
		return err
	}
	var res []error

	// path param id
	if err := r.SetPathParam("id", swag.FormatInt64(o.ID)); err != nil {
		return err
	}

	if o.Page != nil {

		// query param page
		var qrPage int32
		if o.Page != nil {
			qrPage = *o.Page
		}
		qPage := swag.FormatInt32(qrPage)
		if qPage != "" {
			if err := r.SetQueryParam("page", qPage); err != nil {
				return err
			}
		}

	}

	if o.PageSize != nil {

		// query param page_size
		var qrPageSize int32
		if o.PageSize != nil {
			qrPageSize = *o.PageSize
		}
		qPageSize := swag.FormatInt32(qrPageSize)
		if qPageSize != "" {
			if err := r.SetQueryParam("page_size", qPageSize); err != nil {
				return err
			}
		}

	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}
