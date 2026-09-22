// Filename: components/templates/TemplateList.tsx
"use client";

import { useState, useEffect, useCallback } from 'react';
import Link from 'next/link';
import { toast } from "sonner";
import * as api from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { HardDrive, Pencil, Plus, MoreHorizontal, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { NewRenderDialog } from '@/components/render/NewRenderDialog';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog';

interface TemplateListProps {
  projectId: string;
}

export function TemplateList({ projectId }: TemplateListProps) {
  const [templates, setTemplates] = useState<api.Template[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [itemToDelete, setItemToDelete] = useState<api.Template | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);

  const fetchTemplates = useCallback(async () => {
    if (!projectId) return;
    setIsLoading(true);
    setError(null);
    try {
      const response = await api.listTemplates(projectId);
      setTemplates(response.data || []);
    } catch (err) {
      setError("Failed to load templates.");
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    fetchTemplates();
  }, [fetchTemplates]);

  const handleConfirmDelete = async () => {
    if (!itemToDelete) return;
    setIsDeleting(true);
    try {
      await api.deleteTemplate(itemToDelete.uuid);
      toast.success(`Template "${itemToDelete.id}" deleted successfully.`);
      setItemToDelete(null);
      await fetchTemplates(); // Refresh list
    } catch (err) {
      toast.error("Failed to delete template.");
      console.error(err);
    } finally {
      setIsDeleting(false);
    }
  };

  // FIX: Use the skeleton component for the loading state
  if (isLoading) {
    return <TemplateListSkeleton />;
  }

  if (error) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Project Templates</CardTitle>
          <CardDescription>Templates available for generating videos.</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-red-500 text-center py-10">{error}</p>
        </CardContent>
      </Card>
    );
  }

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle>Project Templates</CardTitle>
            <CardDescription>Templates available for generating videos.</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Link href={`/projects/${projectId}/templates/new`}>
              <Button variant="outline">
                <Plus className="mr-2 h-4 w-4" />
                New Template
              </Button>
            </Link>
            <NewRenderDialog projectId={projectId} templates={templates} />
          </div>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[50px]"></TableHead>
                <TableHead>Template ID</TableHead>
                <TableHead>Type</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {templates.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} className="h-24 text-center">
                    No custom templates found. Use the &lsquo;New Template&lsquo; button to create one.
                  </TableCell>
                </TableRow>
              ) : (
                templates.map((template) => (
                  <TableRow key={template.uuid}>
                    <TableCell><HardDrive className="h-5 w-5 text-muted-foreground" /></TableCell>
                    <TableCell className="font-medium">{template.id}</TableCell>
                    <TableCell>
                      {template.is_system ? <Badge variant="secondary">System</Badge> : <Badge variant="outline">Custom</Badge>}
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon"><MoreHorizontal className="h-4 w-4" /></Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem asChild>
                            <Link href={`/projects/${projectId}/templates/${template.uuid}`} className={template.is_system ? "pointer-events-none text-muted-foreground" : ""}>
                              <Pencil className="mr-2 h-4 w-4" />
                              <span>Edit</span>
                            </Link>
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => setItemToDelete(template)} disabled={template.is_system} className="text-red-600 focus:text-red-600">
                            <Trash2 className="mr-2 h-4 w-4" />
                            <span>Delete</span>
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <AlertDialog open={!!itemToDelete} onOpenChange={(open) => !open && setItemToDelete(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Are you sure?</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. This will permanently delete the custom template <span className="font-bold">{itemToDelete?.id}</span>.
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

function TemplateListSkeleton() {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <Skeleton className="h-7 w-48" />
          <Skeleton className="h-4 w-64 mt-2" />
        </div>
        <div className="flex items-center gap-2">
          <Skeleton className="h-10 w-32" />
          <Skeleton className="h-10 w-32" />
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      </CardContent>
    </Card>
  );
}