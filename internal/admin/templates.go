package admin

// Additional template functions for the admin interface

func getPendingVenuesTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Pending Venues - HappyCow Validation</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f7fa; }
        .container { max-width: 1400px; margin: 0 auto; padding: 20px; }
        .header { background: #2c3e50; color: white; padding: 20px 0; margin-bottom: 30px; }
        .header h1 { text-align: center; font-size: 2.5em; }
        .nav { display: flex; gap: 20px; margin-bottom: 30px; }
        .nav a { padding: 10px 20px; background: white; color: #2c3e50; text-decoration: none; border-radius: 5px; }
        .nav a.active, .nav a:hover { background: #3498db; color: white; }
        .filters { background: white; padding: 20px; border-radius: 8px; margin-bottom: 30px; }
        .filters form { display: flex; gap: 15px; align-items: center; flex-wrap: wrap; }
        .filters input, .filters select { padding: 8px 12px; border: 1px solid #ddd; border-radius: 4px; }
        .btn { display: inline-block; padding: 8px 16px; background: #3498db; color: white; text-decoration: none; border-radius: 4px; border: none; cursor: pointer; }
        .btn:hover { background: #2980b9; }
        .btn-sm { padding: 4px 8px; font-size: 12px; }
        .btn-success { background: #27ae60; }
        .btn-danger { background: #e74c3c; }
        .btn-warning { background: #f39c12; }
        .section { background: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
        .table { width: 100%; border-collapse: collapse; }
        .table th, .table td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        .table th { background: #f8f9fa; font-weight: 600; position: sticky; top: 0; }
        .venue-row { cursor: pointer; }
        .venue-row:hover { background: #f8f9fa; }
        .venue-details { display: none; background: #f8f9fa; }
        .venue-details.expanded { display: table-row; }
        .status-badge { padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: bold; }
        .status-approved { background: #d4edda; color: #155724; }
        .status-rejected { background: #f8d7da; color: #721c24; }
        .status-pending { background: #fff3cd; color: #856404; }
        .pagination { display: flex; justify-content: center; gap: 10px; margin-top: 20px; }
        .pagination a { padding: 8px 12px; background: white; border: 1px solid #ddd; color: #333; text-decoration: none; }
        .pagination a.active { background: #3498db; color: white; }
        .batch-controls { margin-bottom: 20px; padding: 15px; background: #fff3cd; border-radius: 5px; }
        .selected-count { font-weight: bold; color: #856404; }
        .checkbox { margin-right: 10px; }
        .actions-column { white-space: nowrap; }
        @media (max-width: 768px) {
            .table { font-size: 14px; }
            .filters form { flex-direction: column; align-items: stretch; }
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="container">
            <h1>📋 Pending Venues</h1>
        </div>
    </div>
    
    <div class="container">
        <nav class="nav">
            <a href="/">Dashboard</a>
            <a href="/venues/pending" class="active">Pending Venues</a>
            <a href="/validation/history">History</a>
            <a href="/analytics">Analytics</a>
        </nav>
        
        <div class="filters">
            <form method="GET">
                <input type="text" name="search" value="{{.Search}}" placeholder="Search venues...">
                <select name="status">
                    <option value="">All Status</option>
                    <option value="pending" {{if eq .Status "pending"}}selected{{end}}>Pending</option>
                    <option value="approved" {{if eq .Status "approved"}}selected{{end}}>Approved</option>
                    <option value="rejected" {{if eq .Status "rejected"}}selected{{end}}>Rejected</option>
                    <option value="manual_review" {{if eq .Status "manual_review"}}selected{{end}}>Manual Review</option>
                </select>
                <button type="submit" class="btn">Filter</button>
                <a href="/venues/pending" class="btn">Clear</a>
            </form>
        </div>
        
        <div class="batch-controls" id="batch-controls" style="display: none;">
            <div class="selected-count" id="selected-count">0 venues selected</div>
            <div style="margin-top: 10px;">
                <button class="btn btn-success" onclick="batchApprove()">✅ Approve Selected</button>
                <button class="btn btn-danger" onclick="batchReject()">❌ Reject Selected</button>
                <button class="btn btn-warning" onclick="batchManualReview()">👀 Manual Review</button>
                <button class="btn" onclick="selectAll()">Select All</button>
                <button class="btn" onclick="selectNone()">Select None</button>
            </div>
        </div>
        
        <div class="section">
            <h2>Venues ({{.Total}} total, Page {{.Page}} of {{.TotalPages}})</h2>
            <table class="table">
                <thead>
                    <tr>
                        <th><input type="checkbox" id="select-all" onchange="toggleSelectAll()"></th>
                        <th>ID</th>
                        <th>Name</th>
                        <th>Location</th>
                        <th>Submitter</th>
                        <th>Authority</th>
                        <th>Status</th>
                        <th>Actions</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .Venues}}
                    <tr class="venue-row" onclick="toggleVenueDetails({{.Venue.ID}})">
                        <td><input type="checkbox" class="venue-checkbox" value="{{.Venue.ID}}" onclick="event.stopPropagation(); updateBatchControls()"></td>
                        <td>{{.Venue.ID}}</td>
                        <td><strong>{{.Venue.Name}}</strong></td>
                        <td>{{.Venue.Location}}</td>
                        <td>{{.User.Username}}</td>
                        <td>
                            {{if .User.Trusted}}<span title="Trusted User">✅</span>{{end}}
                            {{if .IsVenueAdmin}}<span title="Venue Owner">👑</span>{{end}}
                            {{if .AmbassadorLevel}}<span title="Ambassador">🌟</span>{{end}}
                        </td>
                        <td>
                            {{if eq .Venue.Active 1}}
                                <span class="status-badge status-approved">Approved</span>
                            {{else if eq .Venue.Active -1}}
                                <span class="status-badge status-rejected">Rejected</span>
                            {{else}}
                                <span class="status-badge status-pending">Pending</span>
                            {{end}}
                        </td>
                        <td class="actions-column">
                            <a href="/venues/{{.Venue.ID}}" class="btn btn-sm" onclick="event.stopPropagation()">View</a>
                            <button class="btn btn-success btn-sm" onclick="event.stopPropagation(); approveVenue({{.Venue.ID}})">✅</button>
                            <button class="btn btn-danger btn-sm" onclick="event.stopPropagation(); rejectVenue({{.Venue.ID}})">❌</button>
                        </td>
                    </tr>
                    <tr class="venue-details" id="details-{{.Venue.ID}}">
                        <td colspan="8">
                            <div style="padding: 15px;">
                                <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 15px;">
                                    <div><strong>Phone:</strong> {{if .Venue.Phone}}{{.Venue.Phone}}{{else}}N/A{{end}}</div>
                                    <div><strong>Website:</strong> {{if .Venue.URL}}<a href="{{.Venue.URL}}" target="_blank">{{.Venue.URL}}</a>{{else}}N/A{{end}}</div>
                                    <div><strong>Vegan Level:</strong> {{.Venue.Vegan}}/{{.Venue.VegOnly}}</div>
                                    <div><strong>Created:</strong> {{.Venue.CreatedAt.Format "2006-01-02"}}</div>
                                </div>
                                {{if .Venue.AdditionalInfo}}
                                    <div style="margin-top: 10px;"><strong>Description:</strong></div>
                                    <div style="background: #f8f9fa; padding: 10px; border-radius: 4px; margin-top: 5px;">{{.Venue.AdditionalInfo}}</div>
                                {{end}}
                            </div>
                        </td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        
        <div class="pagination">
            {{if gt .Page 1}}
                <a href="?page={{add .Page -1}}&status={{.Status}}&search={{.Search}}">« Previous</a>
            {{end}}
            
            {{range $i := seq 1 .TotalPages}}
                {{if eq $i $.Page}}
                    <a href="#" class="active">{{$i}}</a>
                {{else if or (le $i 3) (ge $i (add $.TotalPages -2)) (and (ge $i (add $.Page -1)) (le $i (add $.Page 1)))}}
                    <a href="?page={{$i}}&status={{$.Status}}&search={{$.Search}}">{{$i}}</a>
                {{end}}
            {{end}}
            
            {{if lt .Page .TotalPages}}
                <a href="?page={{add .Page 1}}&status={{.Status}}&search={{.Search}}">Next »</a>
            {{end}}
        </div>
    </div>
    
    <script>
        function toggleVenueDetails(venueId) {
            const details = document.getElementById('details-' + venueId);
            details.classList.toggle('expanded');
        }
        
        function updateBatchControls() {
            const checkboxes = document.querySelectorAll('.venue-checkbox:checked');
            const count = checkboxes.length;
            const controls = document.getElementById('batch-controls');
            const countElement = document.getElementById('selected-count');
            
            if (count > 0) {
                controls.style.display = 'block';
                countElement.textContent = count + ' venue' + (count === 1 ? '' : 's') + ' selected';
            } else {
                controls.style.display = 'none';
            }
        }
        
        function toggleSelectAll() {
            const selectAll = document.getElementById('select-all');
            const checkboxes = document.querySelectorAll('.venue-checkbox');
            
            checkboxes.forEach(checkbox => {
                checkbox.checked = selectAll.checked;
            });
            
            updateBatchControls();
        }
        
        function selectAll() {
            document.querySelectorAll('.venue-checkbox').forEach(cb => cb.checked = true);
            updateBatchControls();
        }
        
        function selectNone() {
            document.querySelectorAll('.venue-checkbox').forEach(cb => cb.checked = false);
            document.getElementById('select-all').checked = false;
            updateBatchControls();
        }
        
        function getSelectedIds() {
            const checkboxes = document.querySelectorAll('.venue-checkbox:checked');
            return Array.from(checkboxes).map(cb => cb.value);
        }
        
        function batchApprove() {
            const ids = getSelectedIds();
            if (ids.length === 0) return;
            
            const reason = prompt('Reason for batch approval (optional):') || 'Batch approval';
            batchOperation('approve', ids, reason);
        }
        
        function batchReject() {
            const ids = getSelectedIds();
            if (ids.length === 0) return;
            
            const reason = prompt('Reason for batch rejection:');
            if (!reason) return;
            
            batchOperation('reject', ids, reason);
        }
        
        function batchManualReview() {
            const ids = getSelectedIds();
            if (ids.length === 0) return;
            
            const reason = prompt('Reason for manual review (optional):') || 'Requires manual review';
            batchOperation('manual_review', ids, reason);
        }
        
        function batchOperation(action, ids, reason) {
            const formData = new FormData();
            formData.append('action', action);
            formData.append('venue_ids', ids.join(','));
            formData.append('reason', reason);
            
            fetch('/batch-operation', {
                method: 'POST',
                body: formData
            })
            .then(response => response.json())
            .then(data => {
                alert(data.action + ' completed: ' + data.success_count + '/' + data.total_count + ' venues processed');
                location.reload();
            })
            .catch(error => {
                console.error('Error:', error);
                alert('Error performing batch operation');
            });
        }
        
        function approveVenue(id) {
            if (confirm('Approve this venue?')) {
                const reason = prompt('Reason (optional):') || 'Manual approval';
                updateVenueStatus(id, 'approve', reason);
            }
        }
        
        function rejectVenue(id) {
            const reason = prompt('Reason for rejection:');
            if (reason) {
                updateVenueStatus(id, 'reject', reason);
            }
        }
        
        function updateVenueStatus(id, action, reason) {
            const formData = new FormData();
            formData.append(action === 'approve' ? 'notes' : 'reason', reason);
            
            const url = '/venues/' + id + '/' + action;
            
            fetch(url, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: formData
            })
            .then(response => response.json())
            .then(data => {
                // Update the row without full page reload
                location.reload();
            })
            .catch(error => {
                console.error('Error:', error);
                location.reload();
            });
        }
    </script>
</body>
</html>`
}

func getVenueDetailTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Venue Details - {{.Venue.Venue.Name}}</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f7fa; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: #2c3e50; color: white; padding: 20px 0; margin-bottom: 30px; }
        .nav { display: flex; gap: 20px; margin-bottom: 30px; }
        .nav a { padding: 10px 20px; background: white; color: #2c3e50; text-decoration: none; border-radius: 5px; }
        .nav a:hover { background: #3498db; color: white; }
        .venue-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 30px; }
        .section { background: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .section h2 { color: #2c3e50; margin-bottom: 15px; }
        .field-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 15px; }
        .field { margin-bottom: 15px; }
        .field-label { font-weight: bold; color: #666; margin-bottom: 5px; }
        .field-value { color: #333; }
        .btn { display: inline-block; padding: 12px 24px; background: #3498db; color: white; text-decoration: none; border-radius: 5px; border: none; cursor: pointer; margin-right: 10px; }
        .btn-success { background: #27ae60; }
        .btn-danger { background: #e74c3c; }
        .btn-warning { background: #f39c12; }
        .status-badge { padding: 6px 12px; border-radius: 4px; font-weight: bold; }
        .status-approved { background: #d4edda; color: #155724; }
        .status-rejected { background: #f8d7da; color: #721c24; }
        .status-pending { background: #fff3cd; color: #856404; }
        .history-table { width: 100%; border-collapse: collapse; }
        .history-table th, .history-table td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        .history-table th { background: #f8f9fa; }
        .similar-venue { padding: 10px; border: 1px solid #ddd; border-radius: 5px; margin-bottom: 10px; }
        .similar-venue:hover { background: #f8f9fa; }
        .action-form { margin-top: 20px; padding: 15px; background: #f8f9fa; border-radius: 5px; }
        .action-form textarea { width: 100%; padding: 10px; border: 1px solid #ddd; border-radius: 4px; resize: vertical; }
        @media (max-width: 768px) {
            .venue-grid { grid-template-columns: 1fr; }
            .field-grid { grid-template-columns: 1fr; }
        }
    </style>
</head>
<body>
    <div class="header">
        <div class="container">
            <h1>🌱 {{.Venue.Venue.Name}}</h1>
            <p>Venue ID: {{.Venue.Venue.ID}}</p>
        </div>
    </div>
    
    <div class="container">
        <nav class="nav">
            <a href="/">Dashboard</a>
            <a href="/venues/pending">Pending Venues</a>
            <a href="/validation/history">History</a>
            <a href="#" onclick="history.back()">← Back</a>
        </nav>
        
        <div class="venue-grid">
            <div>
                <div class="section">
                    <h2>Venue Information</h2>
                    <div class="field-grid">
                        <div class="field">
                            <div class="field-label">Name</div>
                            <div class="field-value"><strong>{{.Venue.Venue.Name}}</strong></div>
                        </div>
                        <div class="field">
                            <div class="field-label">Location</div>
                            <div class="field-value">{{.Venue.Venue.Location}}</div>
                        </div>
                        <div class="field">
                            <div class="field-label">Phone</div>
                            <div class="field-value">{{if .Venue.Venue.Phone}}{{.Venue.Venue.Phone}}{{else}}N/A{{end}}</div>
                        </div>
                        <div class="field">
                            <div class="field-label">Website</div>
                            <div class="field-value">
                                {{if .Venue.Venue.URL}}
                                    <a href="{{.Venue.Venue.URL}}" target="_blank">{{.Venue.Venue.URL}}</a>
                                {{else}}
                                    N/A
                                {{end}}
                            </div>
                        </div>
                        <div class="field">
                            <div class="field-label">Vegan Level</div>
                            <div class="field-value">{{.Venue.Venue.Vegan}}/{{.Venue.Venue.VegOnly}}</div>
                        </div>
                        <div class="field">
                            <div class="field-label">Date Added</div>
                            <div class="field-value">{{.Venue.Venue.CreatedAt.Format "2006-01-02 15:04"}}</div>
                        </div>
                    </div>
                    
                    {{if .Venue.Venue.AdditionalInfo}}
                        <div class="field">
                            <div class="field-label">Description</div>
                            <div class="field-value" style="background: #f8f9fa; padding: 15px; border-radius: 5px; margin-top: 10px;">
                                {{.Venue.Venue.AdditionalInfo}}
                            </div>
                        </div>
                    {{end}}
                </div>
                
                <div class="section">
                    <h2>Submitter Information</h2>
                    <div class="field-grid">
                        <div class="field">
                            <div class="field-label">Username</div>
                            <div class="field-value">{{.Venue.User.Username}}</div>
                        </div>
                        <div class="field">
                            <div class="field-label">Authority Level</div>
                            <div class="field-value">
                                {{if .Venue.User.Trusted}}<span style="color: green;">✅ Trusted User</span><br>{{end}}
                                {{if .Venue.IsVenueAdmin}}<span style="color: gold;">👑 Venue Owner</span><br>{{end}}
                                {{if .Venue.AmbassadorLevel}}<span style="color: blue;">🌟 Ambassador (Level {{.Venue.AmbassadorLevel}})</span>{{end}}
                                {{if not (or .Venue.User.Trusted .Venue.IsVenueAdmin .Venue.AmbassadorLevel)}}<span style="color: #666;">Regular User</span>{{end}}
                            </div>
                        </div>
                    </div>
                </div>
                
                {{if .History}}
                <div class="section">
                    <h2>Validation History</h2>
                    <table class="history-table">
                        <thead>
                            <tr>
                                <th>Date</th>
                                <th>Action</th>
                                <th>Score</th>
                                <th>Notes</th>
                                <th>Reviewer</th>
                            </tr>
                        </thead>
                        <tbody>
                            {{range .History}}
                            <tr>
                                <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
                                <td>
                                    {{if eq .Status "approved"}}
                                        <span class="status-badge status-approved">Approved</span>
                                    {{else if eq .Status "rejected"}}
                                        <span class="status-badge status-rejected">Rejected</span>
                                    {{else}}
                                        <span class="status-badge status-pending">Review</span>
                                    {{end}}
                                </td>
                                <td>{{.Score}}</td>
                                <td>{{.Notes}}</td>
                                <td>{{if .ReviewedBy}}{{.ReviewedBy}}{{else}}System{{end}}</td>
                            </tr>
                            {{end}}
                        </tbody>
                    </table>
                </div>
                {{end}}
            </div>
            
            <div>
                <div class="section">
                    <h2>Current Status</h2>
                    <div style="text-align: center; margin-bottom: 20px;">
                        {{if eq .Venue.Venue.Active 1}}
                            <span class="status-badge status-approved" style="font-size: 18px;">✅ APPROVED</span>
                        {{else if eq .Venue.Venue.Active -1}}
                            <span class="status-badge status-rejected" style="font-size: 18px;">❌ REJECTED</span>
                        {{else}}
                            <span class="status-badge status-pending" style="font-size: 18px;">⏳ PENDING</span>
                        {{end}}
                    </div>
                    
                    {{if eq .Venue.Venue.Active 0}}
                    <div class="action-form">
                        <h3>Manual Review</h3>
                        <form id="approval-form">
                            <div style="margin-bottom: 15px;">
                                <button type="button" class="btn btn-success" onclick="approveVenue()">✅ Approve</button>
                                <button type="button" class="btn btn-danger" onclick="rejectVenue()">❌ Reject</button>
                            </div>
                            <div style="margin-bottom: 10px;">
                                <label for="notes">Notes/Reason:</label>
                            </div>
                            <textarea id="notes" placeholder="Enter your reason or notes..." rows="3"></textarea>
                        </form>
                    </div>
                    {{end}}
                </div>
                
                {{if .SimilarVenues}}
                <div class="section">
                    <h2>Similar Venues</h2>
                    {{range .SimilarVenues}}
                    <div class="similar-venue">
                        <div><strong>{{.Name}}</strong></div>
                        <div style="color: #666; font-size: 14px;">{{.Location}}</div>
                        {{if .Phone}}<div style="color: #666; font-size: 14px;">{{.Phone}}</div>{{end}}
                    </div>
                    {{end}}
                </div>
                {{end}}
                
                <div class="section">
                    <h2>Quick Actions</h2>
                    <a href="/venues/pending" class="btn">← Back to List</a>
                    <a href="/venues/{{.Venue.Venue.ID}}/edit" class="btn">Edit Venue</a>
                    <button class="btn btn-warning" onclick="flagForReview()">🚩 Flag Issue</button>
                </div>
            </div>
        </div>
    </div>
    
    <script>
        function approveVenue() {
            const notes = document.getElementById('notes').value;
            if (confirm('Approve this venue?')) {
                updateVenueStatus('approve', notes || 'Manual approval');
            }
        }
        
        function rejectVenue() {
            const notes = document.getElementById('notes').value;
            if (!notes.trim()) {
                alert('Please provide a reason for rejection.');
                return;
            }
            if (confirm('Reject this venue?')) {
                updateVenueStatus('reject', notes);
            }
        }
        
        function updateVenueStatus(action, notes) {
            const formData = new FormData();
            formData.append(action === 'approve' ? 'notes' : 'reason', notes);
            
            fetch('/venues/{{.Venue.Venue.ID}}/' + action, {
                method: 'POST',
                body: formData
            })
            .then(response => {
                if (response.ok) {
                    alert('Venue ' + action + 'd successfully!');
                    location.reload();
                } else {
                    alert('Error updating venue status');
                }
            })
            .catch(error => {
                console.error('Error:', error);
                alert('Error updating venue status');
            });
        }
        
        function flagForReview() {
            const reason = prompt('Reason for flagging this venue:');
            if (reason) {
                // Implement flag functionality
                alert('Venue flagged for additional review: ' + reason);
            }
        }
    </script>
</body>
</html>`
}

func getValidationHistoryTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Validation History - HappyCow</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f7fa; }
        .container { max-width: 1400px; margin: 0 auto; padding: 20px; }
        .header { background: #2c3e50; color: white; padding: 20px 0; margin-bottom: 30px; }
        .nav { display: flex; gap: 20px; margin-bottom: 30px; }
        .nav a { padding: 10px 20px; background: white; color: #2c3e50; text-decoration: none; border-radius: 5px; }
        .nav a.active, .nav a:hover { background: #3498db; color: white; }
        .section { background: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
        .table { width: 100%; border-collapse: collapse; }
        .table th, .table td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        .table th { background: #f8f9fa; font-weight: 600; position: sticky; top: 0; }
        .status-badge { padding: 4px 8px; border-radius: 4px; font-size: 12px; font-weight: bold; }
        .status-approved { background: #d4edda; color: #155724; }
        .status-rejected { background: #f8d7da; color: #721c24; }
        .status-pending { background: #fff3cd; color: #856404; }
        .pagination { display: flex; justify-content: center; gap: 10px; margin-top: 20px; }
        .pagination a { padding: 8px 12px; background: white; border: 1px solid #ddd; color: #333; text-decoration: none; }
        .pagination a.active { background: #3498db; color: white; }
        .score-badge { padding: 4px 8px; border-radius: 4px; font-weight: bold; }
        .score-high { background: #d4edda; color: #155724; }
        .score-medium { background: #fff3cd; color: #856404; }
        .score-low { background: #f8d7da; color: #721c24; }
        .expandable-notes { cursor: pointer; max-width: 200px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .expandable-notes:hover { background: #f8f9fa; }
    </style>
</head>
<body>
    <div class="header">
        <div class="container">
            <h1>📋 Validation History</h1>
        </div>
    </div>
    
    <div class="container">
        <nav class="nav">
            <a href="/">Dashboard</a>
            <a href="/venues/pending">Pending Venues</a>
            <a href="/validation/history" class="active">History</a>
            <a href="/analytics">Analytics</a>
        </nav>
        
        <div class="section">
            <h2>Validation History ({{.Total}} total records, Page {{.Page}} of {{.TotalPages}})</h2>
            <table class="table">
                <thead>
                    <tr>
                        <th>Date</th>
                        <th>Venue ID</th>
                        <th>Venue Name</th>
                        <th>Action</th>
                        <th>Score</th>
                        <th>Notes</th>
                        <th>Reviewer</th>
                        <th>Processing Time</th>
                    </tr>
                </thead>
                <tbody>
                    {{range .History}}
                    <tr>
                        <td>{{.CreatedAt.Format "2006-01-02 15:04"}}</td>
                        <td><a href="/venues/{{.VenueID}}">{{.VenueID}}</a></td>
                        <td>{{.VenueName}}</td>
                        <td>
                            {{if eq .Status "approved"}}
                                <span class="status-badge status-approved">Approved</span>
                            {{else if eq .Status "rejected"}}
                                <span class="status-badge status-rejected">Rejected</span>
                            {{else}}
                                <span class="status-badge status-pending">Review</span>
                            {{end}}
                        </td>
                        <td>
                            {{if ge .Score 85}}
                                <span class="score-badge score-high">{{.Score}}</span>
                            {{else if ge .Score 50}}
                                <span class="score-badge score-medium">{{.Score}}</span>
                            {{else}}
                                <span class="score-badge score-low">{{.Score}}</span>
                            {{end}}
                        </td>
                        <td>
                            <div class="expandable-notes" onclick="toggleNotes(this)" title="Click to expand">
                                {{.Notes}}
                            </div>
                        </td>
                        <td>{{if .ReviewedBy}}{{.ReviewedBy}}{{else}}System{{end}}</td>
                        <td>{{if .ProcessingTimeMs}}{{.ProcessingTimeMs}}ms{{else}}N/A{{end}}</td>
                    </tr>
                    {{end}}
                </tbody>
            </table>
        </div>
        
        <div class="pagination">
            {{if gt .Page 1}}
                <a href="?page={{add .Page -1}}">« Previous</a>
            {{end}}
            
            {{range $i := seq 1 .TotalPages}}
                {{if eq $i $.Page}}
                    <a href="#" class="active">{{$i}}</a>
                {{else if or (le $i 3) (ge $i (add $.TotalPages -2)) (and (ge $i (add $.Page -1)) (le $i (add $.Page 1)))}}
                    <a href="?page={{$i}}">{{$i}}</a>
                {{end}}
            {{end}}
            
            {{if lt .Page .TotalPages}}
                <a href="?page={{add .Page 1}}">Next »</a>
            {{end}}
        </div>
    </div>
    
    <script>
        function toggleNotes(element) {
            if (element.style.whiteSpace === 'normal') {
                element.style.whiteSpace = 'nowrap';
                element.style.overflow = 'hidden';
                element.style.textOverflow = 'ellipsis';
                element.style.maxWidth = '200px';
            } else {
                element.style.whiteSpace = 'normal';
                element.style.overflow = 'visible';
                element.style.textOverflow = 'inherit';
                element.style.maxWidth = 'none';
            }
        }
    </script>
</body>
</html>`
}

func getAnalyticsTemplate() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Analytics - HappyCow Validation</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f5f7fa; }
        .container { max-width: 1400px; margin: 0 auto; padding: 20px; }
        .header { background: #2c3e50; color: white; padding: 20px 0; margin-bottom: 30px; }
        .nav { display: flex; gap: 20px; margin-bottom: 30px; }
        .nav a { padding: 10px 20px; background: white; color: #2c3e50; text-decoration: none; border-radius: 5px; }
        .nav a.active, .nav a:hover { background: #3498db; color: white; }
        .metrics-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; margin-bottom: 30px; }
        .metric-card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .metric-title { font-size: 18px; font-weight: bold; color: #2c3e50; margin-bottom: 10px; }
        .metric-value { font-size: 32px; font-weight: bold; color: #3498db; }
        .metric-subtitle { color: #666; margin-top: 5px; font-size: 14px; }
        .section { background: white; padding: 20px; border-radius: 8px; margin-bottom: 20px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .progress-bar { width: 100%; height: 20px; background: #ecf0f1; border-radius: 10px; overflow: hidden; margin: 10px 0; }
        .progress-fill { height: 100%; background: linear-gradient(90deg, #3498db, #2ecc71); }
        .stat-row { display: flex; justify-content: space-between; align-items: center; padding: 10px 0; border-bottom: 1px solid #eee; }
        .stat-row:last-child { border-bottom: none; }
        .cost-breakdown { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 15px; }
        .cost-item { text-align: center; padding: 15px; background: #f8f9fa; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="header">
        <div class="container">
            <h1>📊 Analytics Dashboard</h1>
        </div>
    </div>
    
    <div class="container">
        <nav class="nav">
            <a href="/">Dashboard</a>
            <a href="/venues/pending">Pending Venues</a>
            <a href="/validation/history">History</a>
            <a href="/analytics" class="active">Analytics</a>
        </nav>
        
        <div class="metrics-grid">
            <div class="metric-card">
                <div class="metric-title">🎯 Automation Rate</div>
                <div class="metric-value">{{printf "%.1f%%" .AutomationRate}}</div>
                <div class="metric-subtitle">Venues processed automatically</div>
                <div class="progress-bar">
                    <div class="progress-fill" style="width: {{.AutomationRate}}%;"></div>
                </div>
            </div>
            
            <div class="metric-card">
                <div class="metric-title">💰 Cost Efficiency</div>
                <div class="metric-value">${{printf "%.3f" .CostPerVenue}}</div>
                <div class="metric-subtitle">Average cost per venue</div>
            </div>
            
            <div class="metric-card">
                <div class="metric-title">⚡ Processing Speed</div>
                <div class="metric-value">{{.ProcessingStats.AverageTimeMs}}ms</div>
                <div class="metric-subtitle">Average processing time</div>
            </div>
            
            <div class="metric-card">
                <div class="metric-title">📈 Success Rate</div>
                <div class="metric-value">{{if gt .ProcessingStats.TotalJobs 0}}{{printf "%.1f%%" (mul (div (add .ProcessingStats.SuccessfulJobs 0.0) (add .ProcessingStats.TotalJobs 0.0)) 100)}}{{else}}0.0%{{end}}</div>
                <div class="metric-subtitle">Processing success rate</div>
            </div>
        </div>
        
        <div class="section">
            <h2>Processing Statistics</h2>
            <div class="stat-row">
                <span><strong>Total Venues Processed:</strong></span>
                <span>{{.ProcessingStats.TotalJobs}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Completed:</strong></span>
                <span>{{.ProcessingStats.CompletedJobs}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Successful:</strong></span>
                <span>{{.ProcessingStats.SuccessfulJobs}} ({{if gt .ProcessingStats.TotalJobs 0}}{{printf "%.1f%%" (mul (div (add .ProcessingStats.SuccessfulJobs 0.0) (add .ProcessingStats.TotalJobs 0.0)) 100)}}{{else}}0%{{end}})</span>
            </div>
            <div class="stat-row">
                <span><strong>Failed:</strong></span>
                <span>{{.ProcessingStats.FailedJobs}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Auto-Approved:</strong></span>
                <span style="color: #27ae60;">{{.ProcessingStats.AutoApproved}} ({{if gt .ProcessingStats.TotalJobs 0}}{{printf "%.1f%%" (mul (div (add .ProcessingStats.AutoApproved 0.0) (add .ProcessingStats.TotalJobs 0.0)) 100)}}{{else}}0%{{end}})</span>
            </div>
            <div class="stat-row">
                <span><strong>Auto-Rejected:</strong></span>
                <span style="color: #e74c3c;">{{.ProcessingStats.AutoRejected}} ({{if gt .ProcessingStats.TotalJobs 0}}{{printf "%.1f%%" (mul (div (add .ProcessingStats.AutoRejected 0.0) (add .ProcessingStats.TotalJobs 0.0)) 100)}}{{else}}0%{{end}})</span>
            </div>
            <div class="stat-row">
                <span><strong>Manual Review Required:</strong></span>
                <span style="color: #f39c12;">{{.ProcessingStats.ManualReview}} ({{if gt .ProcessingStats.TotalJobs 0}}{{printf "%.1f%%" (mul (div (add .ProcessingStats.ManualReview 0.0) (add .ProcessingStats.TotalJobs 0.0)) 100)}}{{else}}0%{{end}})</span>
            </div>
        </div>
        
        <div class="section">
            <h2>API Usage & Costs</h2>
            <div class="cost-breakdown">
                <div class="cost-item">
                    <div style="font-size: 24px; font-weight: bold; color: #3498db;">{{.ProcessingStats.APICallsGoogle}}</div>
                    <div>Google Maps API Calls</div>
                </div>
                <div class="cost-item">
                    <div style="font-size: 24px; font-weight: bold; color: #e74c3c;">{{.ProcessingStats.APICallsOpenAI}}</div>
                    <div>OpenAI API Calls</div>
                </div>
                <div class="cost-item">
                    <div style="font-size: 24px; font-weight: bold; color: #27ae60;">${{printf "%.2f" .ProcessingStats.TotalCostUSD}}</div>
                    <div>Total API Costs</div>
                </div>
                <div class="cost-item">
                    <div style="font-size: 24px; font-weight: bold; color: #f39c12;">${{printf "%.4f" .CostPerVenue}}</div>
                    <div>Cost Per Venue</div>
                </div>
            </div>
        </div>
        
        <div class="section">
            <h2>System Performance</h2>
            <div class="stat-row">
                <span><strong>Active Workers:</strong></span>
                <span>{{.ProcessingStats.WorkerCount}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Queue Size:</strong></span>
                <span>{{.ProcessingStats.QueueSize}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Started:</strong></span>
                <span>{{.ProcessingStats.StartTime.Format "2006-01-02 15:04:05"}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Last Activity:</strong></span>
                <span>{{.ProcessingStats.LastActivity.Format "2006-01-02 15:04:05"}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Uptime:</strong></span>
                <span>{{.ProcessingStats.LastActivity.Sub .ProcessingStats.StartTime}}</span>
            </div>
        </div>
        
        {{if .VenueStats}}
        <div class="section">
            <h2>Venue Database Statistics</h2>
            <div class="stat-row">
                <span><strong>Total Venues:</strong></span>
                <span>{{.VenueStats.TotalVenues}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Pending Review:</strong></span>
                <span>{{.VenueStats.PendingVenues}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Approved:</strong></span>
                <span style="color: #27ae60;">{{.VenueStats.ApprovedVenues}}</span>
            </div>
            <div class="stat-row">
                <span><strong>Rejected:</strong></span>
                <span style="color: #e74c3c;">{{.VenueStats.RejectedVenues}}</span>
            </div>
        </div>
        {{end}}
    </div>
    
    <script>
        // Auto-refresh every 60 seconds
        setInterval(function() {
            location.reload();
        }, 60000);
    </script>
</body>
</html>`
}
