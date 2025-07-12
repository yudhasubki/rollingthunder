export type TabKind = 'query' | 'table';

export interface Tab {
  id:           string;              
  title:        string;           
  kind:         TabKind;           
  schema?:      string;         
  table?:       string;         
  sql?:         string;         
  columns?:     any[];        
  indices?:     any[];        
  status?:      string;       
  level?:       'info' | 'warn' | 'error';
  createdAt:    number;
}