using System;
using System.Collections.Generic;

namespace Glyphchain.Sharp
{
    public interface IProvider<T>
    {
        int Value { get; }
    }

    public enum Color
    {
        Red,
        Green
    }

    public class Box<T>
    {
        public T Content;
    }

    public class Outer
    {
        public class Item
        {
            public int Size;
        }
    }

    public class Renderer : IProvider<int>, IDisposable
    {
        public const int A = 1, B = 2;

        private Dictionary<string, int> map;

        private (int, int) pair;

        public event EventHandler Changed;

        public event EventHandler Moved { add { } remove { } }

        public Color Color { get; }

        public string Title { get; set; }

        public void Draw(int count)
        {
        }

        public void Draw(string text)
        {
        }

        public void Fill(int count)
        {
        }

        int IProvider<int>.Value { get { return 0; } }

        void IDisposable.Dispose()
        {
        }
    }
}
