DROP TRIGGER IF EXISTS trg_knowledge_graph_deletions_no_truncate
    ON public.knowledge_graph_deletion_signals;
DROP TRIGGER IF EXISTS trg_knowledge_graph_deletions_immutable
    ON public.knowledge_graph_deletion_signals;
DROP TRIGGER IF EXISTS trg_knowledge_graph_conflicts_no_truncate
    ON public.knowledge_graph_conflict_records;
DROP TRIGGER IF EXISTS trg_knowledge_graph_conflicts_immutable
    ON public.knowledge_graph_conflict_records;
DROP TRIGGER IF EXISTS trg_knowledge_graph_provenance_no_truncate
    ON public.knowledge_graph_provenance_events;
DROP TRIGGER IF EXISTS trg_knowledge_graph_provenance_immutable
    ON public.knowledge_graph_provenance_events;
DROP TRIGGER IF EXISTS trg_knowledge_graph_edge_revisions_no_truncate
    ON public.knowledge_graph_edge_revisions;
DROP TRIGGER IF EXISTS trg_knowledge_graph_edge_revisions_immutable
    ON public.knowledge_graph_edge_revisions;
DROP TRIGGER IF EXISTS trg_knowledge_graph_node_revisions_no_truncate
    ON public.knowledge_graph_node_revisions;
DROP TRIGGER IF EXISTS trg_knowledge_graph_node_revisions_immutable
    ON public.knowledge_graph_node_revisions;
DROP TRIGGER IF EXISTS trg_knowledge_graph_edges_no_delete
    ON public.knowledge_graph_edges;
DROP TRIGGER IF EXISTS trg_knowledge_graph_nodes_no_delete
    ON public.knowledge_graph_nodes;
DROP TRIGGER IF EXISTS trg_knowledge_graph_nodes_correction_links
    ON public.knowledge_graph_nodes;
DROP TRIGGER IF EXISTS trg_knowledge_graph_edges_update
    ON public.knowledge_graph_edges;
DROP TRIGGER IF EXISTS trg_knowledge_graph_nodes_update
    ON public.knowledge_graph_nodes;

DROP FUNCTION IF EXISTS public.hai_reject_knowledge_graph_history_mutation();
DROP FUNCTION IF EXISTS public.hai_reject_knowledge_graph_physical_delete();
DROP FUNCTION IF EXISTS public.hai_validate_knowledge_graph_correction_links();
DROP FUNCTION IF EXISTS public.hai_enforce_knowledge_graph_current_update();

DROP TABLE IF EXISTS public.knowledge_graph_deletion_signals;
DROP TABLE IF EXISTS public.knowledge_graph_conflict_records;
DROP TABLE IF EXISTS public.knowledge_graph_provenance_events;
DROP TABLE IF EXISTS public.knowledge_graph_edge_revisions;
DROP TABLE IF EXISTS public.knowledge_graph_node_revisions;
DROP TABLE IF EXISTS public.knowledge_graph_edges;
DROP TABLE IF EXISTS public.knowledge_graph_nodes;
