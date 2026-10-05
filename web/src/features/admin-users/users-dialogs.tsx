import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { toast } from "sonner";

import { Loader2 } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/shared/components/ui/alert-dialog";
import { Button } from "@/shared/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/shared/components/ui/dialog";
import { Input } from "@/shared/components/ui/input";
import { createUser, deleteUser, getUserWithETag, updateUser } from "./api";
import { isStaleWrite } from "@/shared/api/core";

import type { User } from "@/shared/types/core";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/shared/components/ui/form";

export const userFormSchema = z.object({
  name: z.string(),
  email: z.string().email("Enter a valid email"),
});

export type UserForm = z.infer<typeof userFormSchema>;

export function UserFormFields({ form }: { form: ReturnType<typeof useForm<UserForm>> }) {
  return (
    <>
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Name</FormLabel>
            <FormControl>
              <Input placeholder="Ada Lovelace" {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="email"
        render={({ field }) => (
          <FormItem>
            <FormLabel>Email</FormLabel>
            <FormControl>
              <Input type="email" placeholder="ada@example.com" {...field} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  );
}

export function CreateDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const form = useForm<UserForm>({
    resolver: zodResolver(userFormSchema),
    defaultValues: { name: "", email: "" },
  });
  const create = useMutation({
    mutationFn: (values: UserForm) => createUser(values.email, values.name),
    onSuccess: (user) => {
      toast.success(`Created ${user.email}`);
      form.reset();
      onOpenChange(false);
      onCreated();
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New user</DialogTitle>
          <DialogDescription>
            Creates a passwordless account, as if provisioned by an external system.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id="create-user"
            onSubmit={form.handleSubmit((values) => create.mutate(values))}
            className="space-y-3"
          >
            <UserFormFields form={form} />
          </form>
        </Form>
        <DialogFooter>
          <Button type="submit" form="create-user" disabled={create.isPending}>
            {create.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function EditDialog({
  user,
  onOpenChange,
  onSaved,
}: {
  user: User | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const queryClient = useQueryClient();
  const form = useForm<UserForm>({
    resolver: zodResolver(userFormSchema),
    values: { name: user?.name ?? "", email: user?.email ?? "" },
  });
  // Optimistic locking: fetch the current ETag when the dialog opens and
  // echo it as If-Match; a concurrent edit fails with 412 instead of
  // clobbering, and the dialog reloads the fresh state.
  const [etag, setEtag] = useState("");
  useEffect(() => {
    if (!user) return;
    let cancelled = false;
    getUserWithETag(user.id)
      .then(({ etag: current }) => {
        if (!cancelled) setEtag(current);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [user]);
  const update = useMutation({
    mutationFn: (values: UserForm) =>
      updateUser(user?.id ?? "", values.email, values.name, undefined, etag),
    onSuccess: () => {
      toast.success("User updated");
      onOpenChange(false);
      onSaved();
    },
    onError: (error) => {
      if (isStaleWrite(error)) {
        toast.error(
          "The user was changed by someone else — the latest version has been loaded; please review and save again.",
        );
        void queryClient.invalidateQueries({ queryKey: ["users"] });
        if (user) {
          getUserWithETag(user.id).then(({ data, etag: fresh }) => {
            setEtag(fresh);
            form.reset({ name: data.name, email: data.email });
          });
        }
        return;
      }
      toast.error(error.message);
    },
  });
  return (
    <Dialog open={user !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit user</DialogTitle>
          <DialogDescription>
            Updates email and name via PUT /api/users/{user?.id}.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id="edit-user"
            onSubmit={form.handleSubmit((values) => update.mutate(values))}
            className="space-y-3"
          >
            <UserFormFields form={form} />
          </form>
        </Form>
        <DialogFooter>
          <Button type="submit" form="edit-user" disabled={update.isPending}>
            {update.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function DeleteDialog({
  user,
  onOpenChange,
  onDeleted,
}: {
  user: User | null;
  onOpenChange: (open: boolean) => void;
  onDeleted: () => void;
}) {
  const remove = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: () => {
      toast.success("User deleted");
      onOpenChange(false);
      onDeleted();
    },
    onError: (error) => toast.error(error.message),
  });
  return (
    <AlertDialog open={user !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {user?.email}?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes the account together with its credentials, identities, and sessions. There
            is no undo.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-white hover:bg-destructive/90"
            onClick={(event) => {
              event.preventDefault();
              if (user) remove.mutate(user.id);
            }}
          >
            {remove.isPending ? <Loader2 className="size-4 animate-spin" /> : null} Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/**
 * UserRolesDialog (shared with the user card) manages the account's role
 * assignments in the caller's active scope (or the platform). Grants map
 * to Casbin g rules.
 */
