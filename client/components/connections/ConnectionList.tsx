// Filename: components/connections/ConnectionList.tsx
"use client";

import { useState, useEffect, useCallback } from 'react';
import * as api from '@/lib/api';
import { Connection } from '@/lib/api';
import { AddConnectionDialog } from './AddConnectionDialog';

// UI Components
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Plug, Trash2, Plus } from 'lucide-react';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

interface ConnectionListProps {
  projectId: string;
}

export function ConnectionList({ projectId }: ConnectionListProps) {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isAddDialogOpen, setIsAddDialogOpen] = useState(false);
  
  const [itemToDelete, setItemToDelete] = useState<Connection | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const fetchConnections = useCallback(async () => {
    if (!projectId) return;
    setIsLoading(true);
    try {
      const response = await api.listConnections(projectId);
      setConnections(response.data || []);
    } catch (err) {
      setError("Failed to load connections.");
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    fetchConnections();
  }, [fetchConnections]);

  const handleConfirmDelete = async () => {
    if (!itemToDelete) return;
    setIsDeleting(true);
    try {
      await api.deleteConnection(itemToDelete.id);
      setItemToDelete(null);
      await fetchConnections(); // Refresh list
    } catch (err) {
      console.error("Failed to delete connection", err);
      setError("Failed to delete connection.");
    } finally {
      setIsDeleting(false);
    }
  };

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Connections</CardTitle>
            <CardDescription>Destinations where your videos will be sent.</CardDescription>
          </div>
          <Button onClick={() => setIsAddDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add Connection
          </Button>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-12 w-full" />
              <Skeleton className="h-12 w-full" />
            </div>
          ) : error ? (
            <p className="text-red-500">{error}</p>
          ) : connections.length === 0 ? (
            <p className="text-sm text-muted-foreground">No connections configured for this project.</p>
          ) : (
            <ul className="space-y-3">
              {connections.map((conn) => (
                <li key={conn.id} className="flex items-center justify-between p-3 bg-muted rounded-md">
                  <div className="flex items-center gap-3">
                    <Plug className="h-5 w-5 text-muted-foreground" />
                    <span className="font-medium capitalize">{conn.platform}</span>
                  </div>
                  <Button variant="ghost" size="icon" onClick={() => setItemToDelete(conn)}>
                    <Trash2 className="h-4 w-4 text-destructive" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <AddConnectionDialog
        projectId={projectId}
        open={isAddDialogOpen}
        onOpenChange={setIsAddDialogOpen}
        onSuccess={fetchConnections}
      />

      <AlertDialog open={!!itemToDelete} onOpenChange={(open) => !open && setItemToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you sure?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete the <span className="font-bold capitalize">{itemToDelete?.platform}</span> connection.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isDeleting}>Cancel</AlertDialogCancel>
            <AlertDialogAction onClick={handleConfirmDelete} disabled={isDeleting} className="bg-destructive hover:bg-destructive/90">
              {isDeleting ? 'Deleting...' : 'Continue'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}