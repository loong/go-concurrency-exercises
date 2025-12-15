# Exercise requirements

You are implementing a DB instance which is used to query data at external DB through a driver called `EmulatedDriver`. 
Your task is to implement `QueryContext`, which must ensure:
1. When the context is timed out or get cancelled, you must return as soon as possible.
2. Before return, ensuring all the resource of the operation is clean up.
3. The operation must return errors if a failure happens.