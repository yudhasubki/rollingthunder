// Query History Store
// Saves executed queries with timestamp, status, and result count

export interface QueryHistoryItem {
    id: string;
    query: string;
    timestamp: Date;
    status: 'success' | 'error';
    rowCount?: number;
    errorMessage?: string;
    executionTime?: number; // ms
}

// Max history items to keep
const MAX_HISTORY = 50;

// Load from localStorage
function loadHistory(): QueryHistoryItem[] {
    if (typeof window === 'undefined') return [];
    try {
        const stored = localStorage.getItem('queryHistory');
        if (stored) {
            const parsed = JSON.parse(stored);
            return parsed.map((item: any) => ({
                ...item,
                timestamp: new Date(item.timestamp)
            }));
        }
    } catch (e) {
        console.error('Failed to load query history:', e);
    }
    return [];
}

// Save to localStorage
function saveHistory(items: QueryHistoryItem[]) {
    if (typeof window === 'undefined') return;
    try {
        localStorage.setItem('queryHistory', JSON.stringify(items));
    } catch (e) {
        console.error('Failed to save query history:', e);
    }
}

// State
let history = $state<QueryHistoryItem[]>(loadHistory());

// Add query to history
export function addQueryToHistory(
    query: string,
    status: 'success' | 'error',
    rowCount?: number,
    errorMessage?: string,
    executionTime?: number
) {
    const item: QueryHistoryItem = {
        id: crypto.randomUUID(),
        query: query.trim(),
        timestamp: new Date(),
        status,
        rowCount,
        errorMessage,
        executionTime
    };

    // Add to start, limit size
    history = [item, ...history].slice(0, MAX_HISTORY);
    saveHistory(history);
}

// Get history
export function getQueryHistory(): QueryHistoryItem[] {
    return history;
}

// Clear history
export function clearQueryHistory() {
    history = [];
    saveHistory([]);
}

// Delete single item
export function deleteQueryHistoryItem(id: string) {
    history = history.filter((item) => item.id !== id);
    saveHistory(history);
}
